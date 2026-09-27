package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

type legacyMCPConfigurationSource struct {
	configs *mcpconfigstore.Store
}

func (s legacyMCPConfigurationSource) GetProviderConfig(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (providerplugin.Configuration, error) {
	if s.configs == nil {
		return providerplugin.Configuration{}, fmt.Errorf(
			"load provider plugin config: MCP config store is unavailable",
		)
	}
	legacy, err := s.configs.Get(ctx, providerID)
	if err != nil {
		return providerplugin.Configuration{}, err
	}
	return legacyMCPProviderConfiguration(legacy)
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
	sum := sha256.Sum256(raw)
	return providerplugin.Configuration{
		ProviderID:    legacy.ProviderID,
		PluginID:      mcpplugin.ID,
		ConfigVersion: mcpplugin.ConfigVersion,
		Revision:      "sha256:" + hex.EncodeToString(sum[:]),
		Config:        raw,
	}, nil
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
