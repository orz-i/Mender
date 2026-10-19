package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"github.com/orz-i/Mender/backend/internal/mcp/infrastructure/headertemplate"
	mcpprovider "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/provider"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

const pluginVersion = "1.0.0"

// Plugin is Mender's built-in upstream MCP Provider plugin.
type Plugin struct {
	lookupEnv func(string) (string, bool)
}

// New constructs the built-in MCP Provider plugin.
func New() *Plugin {
	return &Plugin{lookupEnv: os.LookupEnv}
}

// Descriptor implements providerplugin.Plugin.
func (*Plugin) Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{
		ID:            ID,
		Version:       pluginVersion,
		DisplayName:   "MCP",
		Description:   "Model Context Protocol provider",
		ConfigVersion: ConfigVersion,
	}
}

// ValidateConfig implements providerplugin.Plugin without activation side
// effects.
func (*Plugin) ValidateConfig(
	_ context.Context,
	raw json.RawMessage,
) error {
	_, err := DecodeConfig(raw)
	return err
}

// InspectConfig exposes only bounded platform-governed metadata from MCP
// configuration.
func (*Plugin) InspectConfig(
	_ context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	config, err := DecodeConfig(raw)
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	return providerplugin.NewConfigMetadata(config.RequiredCredentialRefs())
}

// Open resolves connection credentials and opens one MCP client session.
func (p *Plugin) Open(
	ctx context.Context,
	request providerplugin.OpenRequest,
) (providerplugin.Instance, error) {
	if request.ConfigVersion != ConfigVersion {
		return nil, fmt.Errorf(
			"open MCP plugin: unsupported config version %d",
			request.ConfigVersion,
		)
	}
	config, err := DecodeConfig(request.Config)
	if err != nil {
		return nil, fmt.Errorf("open MCP plugin: %w", err)
	}

	switch config.Transport.Kind {
	case TransportStreamableHTTP:
		return p.openHTTP(ctx, request, config)
	case TransportStdio:
		return p.openStdio(ctx, request, config)
	default:
		return nil, fmt.Errorf(
			"open MCP plugin: unsupported transport %q",
			config.Transport.Kind,
		)
	}
}

func (p *Plugin) openHTTP(
	ctx context.Context,
	request providerplugin.OpenRequest,
	config Config,
) (providerplugin.Instance, error) {
	// The non-secret binding is accepted for persistence, but OAuth credentials
	// must never be silently downgraded to an unauthenticated MCP connection.
	if config.Transport.OAuthCredentialRef != "" {
		return nil, fmt.Errorf("open MCP HTTP: OAuth credential is not yet connected to an OAuth token source")
	}
	lookupEnv := p.lookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	headers, err := headertemplate.Resolve(config.Transport.Headers, lookupEnv)
	if err != nil {
		return nil, fmt.Errorf("open MCP HTTP: resolve headers: %w", err)
	}

	for _, binding := range config.Transport.CredentialHeaders {
		value, err := resolveCredentialValue(ctx, request, binding.Reference)
		if err != nil {
			return nil, fmt.Errorf(
				"open MCP HTTP: resolve credential %s: %w",
				binding.Reference,
				err,
			)
		}
		if strings.IndexByte(value, 13) >= 0 ||
			strings.IndexByte(value, 10) >= 0 ||
			strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf(
				"open MCP HTTP: credential header %q contains an invalid value",
				binding.Name,
			)
		}
		if scheme := strings.TrimSpace(binding.Scheme); scheme != "" {
			value = scheme + " " + value
		}
		headers[strings.TrimSpace(binding.Name)] = value
	}

	connection, err := mcpprovider.ConnectHTTP(ctx, mcpprovider.HTTPConfig{
		Endpoint:            config.Transport.Endpoint,
		Headers:             headers,
		RequiredCredentials: config.RequiredCredentialRefs(),
	})
	if err != nil {
		return nil, fmt.Errorf("open MCP HTTP: %w", err)
	}
	return connection, nil
}

func (*Plugin) openStdio(
	ctx context.Context,
	request providerplugin.OpenRequest,
	config Config,
) (providerplugin.Instance, error) {
	environment := make(map[string]string, len(config.Transport.CredentialEnvironment))
	for _, binding := range config.Transport.CredentialEnvironment {
		value, err := resolveCredentialValue(ctx, request, binding.Reference)
		if err != nil {
			return nil, fmt.Errorf(
				"open MCP stdio: resolve credential %s: %w",
				binding.Reference,
				err,
			)
		}
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf(
				"open MCP stdio: credential environment %q contains an invalid value",
				binding.Name,
			)
		}
		environment[strings.TrimSpace(binding.Name)] = value
	}

	connection, err := mcpprovider.ConnectCommand(ctx, mcpprovider.CommandConfig{
		Executable:          config.Transport.Executable,
		Args:                append([]string(nil), config.Transport.Args...),
		Directory:           config.Transport.Directory,
		Environment:         environment,
		RequiredCredentials: config.RequiredCredentialRefs(),
	})
	if err != nil {
		return nil, fmt.Errorf("open MCP stdio: %w", err)
	}
	return connection, nil
}

func resolveCredentialValue(
	ctx context.Context,
	request providerplugin.OpenRequest,
	reference credentialdomain.Reference,
) (string, error) {
	if request.Credentials == nil {
		return "", fmt.Errorf("credential resolver is unavailable")
	}
	secret, err := request.Credentials.Resolve(ctx, reference)
	if err != nil {
		return "", err
	}
	value := string(secret)
	for i := range secret {
		secret[i] = 0
	}
	return value, nil
}

var _ providerplugin.Plugin = (*Plugin)(nil)
