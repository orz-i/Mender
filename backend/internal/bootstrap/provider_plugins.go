package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

const legacyMCPImportName = "legacy-mcp-v3"

func importLegacyMCPProviderConfigs(
	ctx context.Context,
	legacy *mcpconfigstore.Store,
	generic *providerconfigstore.Store,
) error {
	if legacy == nil || generic == nil {
		return fmt.Errorf("import legacy MCP provider configs: stores must be available")
	}
	configs, err := legacy.List(ctx)
	if err != nil {
		return fmt.Errorf("import legacy MCP provider configs: list: %w", err)
	}
	mapped := make([]providerplugin.Configuration, 0, len(configs))
	for _, config := range configs {
		converted, err := legacyMCPProviderConfiguration(config)
		if err != nil {
			return fmt.Errorf(
				"import legacy MCP provider configs: provider %s: %w",
				config.ProviderID,
				err,
			)
		}
		mapped = append(mapped, converted)
	}
	if err := generic.ImportOnce(ctx, legacyMCPImportName, mapped); err != nil {
		return fmt.Errorf("import legacy MCP provider configs: %w", err)
	}
	return nil
}

func legacyMCPProviderConfiguration(
	legacy mcpconfigstore.Config,
) (providerplugin.Configuration, error) {
	config, err := legacyMCPPluginConfig(legacy)
	if err != nil {
		return providerplugin.Configuration{}, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return providerplugin.Configuration{}, fmt.Errorf(
			"map MCP provider config: encode plugin config: %w",
			err,
		)
	}
	configEnvelope, err := providerplugin.NewConfiguration(
		legacy.ProviderID,
		mcpplugin.ID,
		mcpplugin.ConfigVersion,
		raw,
	)
	if err != nil {
		return providerplugin.Configuration{}, fmt.Errorf(
			"map MCP provider config: build provider config: %w",
			err,
		)
	}
	metadata, err := providerplugin.NewConfigMetadata(config.RequiredCredentialRefs())
	if err != nil {
		return providerplugin.Configuration{}, fmt.Errorf(
			"map MCP provider config: metadata: %w",
			err,
		)
	}
	configEnvelope, err = configEnvelope.WithMetadata(metadata)
	if err != nil {
		return providerplugin.Configuration{}, fmt.Errorf(
			"map MCP provider config: attach metadata: %w",
			err,
		)
	}
	return configEnvelope, nil
}

func legacyMCPPluginConfig(
	legacy mcpconfigstore.Config,
) (mcpplugin.Config, error) {
	if err := legacy.Validate(); err != nil {
		return mcpplugin.Config{}, fmt.Errorf("map MCP provider config: %w", err)
	}

	config := mcpplugin.Config{}
	switch legacy.Transport {
	case mcpconfigstore.TransportHTTP:
		config.Transport = mcpplugin.TransportConfig{
			Kind:              mcpplugin.TransportStreamableHTTP,
			Endpoint:          legacy.Endpoint,
			Headers:           cloneStringMap(legacy.Headers),
			CredentialHeaders: mapCredentialHeaders(legacy.CredentialHeaders),
		}
	case mcpconfigstore.TransportStdio:
		config.Transport = mcpplugin.TransportConfig{
			Kind:                  mcpplugin.TransportStdio,
			Executable:            legacy.Executable,
			Args:                  append([]string(nil), legacy.Args...),
			Directory:             legacy.Directory,
			CredentialEnvironment: mapCredentialEnvironment(legacy.CredentialEnvironment),
		}
	default:
		return mcpplugin.Config{}, fmt.Errorf(
			"map MCP provider config: unsupported transport %q",
			legacy.Transport,
		)
	}
	if err := config.Validate(); err != nil {
		return mcpplugin.Config{}, fmt.Errorf("map MCP provider config: %w", err)
	}
	return config, nil
}

func mapCredentialHeaders(
	bindings []mcpconfigstore.CredentialHeader,
) []mcpplugin.CredentialHeader {
	result := make([]mcpplugin.CredentialHeader, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, mcpplugin.CredentialHeader{
			Name:      binding.Name,
			Scheme:    binding.Scheme,
			Reference: binding.Reference,
		})
	}
	return result
}

func mapCredentialEnvironment(
	bindings []mcpconfigstore.CredentialEnvironment,
) []mcpplugin.CredentialEnvironment {
	result := make([]mcpplugin.CredentialEnvironment, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, mcpplugin.CredentialEnvironment{
			Name:      binding.Name,
			Reference: binding.Reference,
		})
	}
	return result
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
