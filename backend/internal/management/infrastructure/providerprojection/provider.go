// Package providerprojection maps plugin-owned configuration into inert,
// management-safe views shared by all management transports.
package providerprojection

import (
	"maps"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	managementquery "github.com/orz-i/Mender/backend/internal/management/application/query"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
)

func Provider(provider catalogdomain.Provider, config *providerplugin.Configuration,
	descriptor *providerplugin.Descriptor, toolCount int) managementquery.ProviderView {
	view := managementquery.ProviderView{
		ProviderID: provider.ID().String(), Namespace: provider.Namespace().String(),
		DisplayName: provider.DisplayName(), Description: provider.Description(),
		CreatedAt: provider.CreatedAt(), UpdatedAt: provider.UpdatedAt(), ToolCount: toolCount,
	}
	if config == nil {
		return view
	}
	configuration := &managementquery.ProviderConfigurationView{
		PluginID: config.PluginID.String(), ConfigVersion: config.ConfigVersion,
	}
	view.Configuration = configuration
	if config.InstallationID != nil {
		configuration.InstallationID = config.InstallationID.String()
	}
	if metadata, ok := config.ConfigMetadata(); ok {
		configuration.ConnectionSummary = metadata.ConnectionSummary
	}
	if descriptor != nil {
		configuration.Plugin = &managementquery.ProviderPluginView{
			Version: descriptor.Version, DisplayName: descriptor.DisplayName,
		}
	}
	if safe, err := providerplugin.ManagementSafeConfig(descriptor, config.Config); err == nil {
		configuration.SafeConfig = safe
		configuration.HasSafeConfig = true
	}
	if config.PluginID != mcpplugin.ID {
		return view
	}
	decoded, err := mcpplugin.DecodeConfig(config.Config)
	if err != nil {
		return view
	}
	connection := &managementquery.ProviderConnectionView{}
	configuration.Connection = connection
	switch decoded.Transport.Kind {
	case mcpplugin.TransportStreamableHTTP:
		connection.Transport = "http"
		connection.Endpoint = decoded.Transport.Endpoint
		connection.Headers = maps.Clone(decoded.Transport.Headers)
		if connection.Headers == nil {
			connection.Headers = map[string]string{}
		}
		connection.OAuthCredentialRef = decoded.Transport.OAuthCredentialRef.String()
		connection.CredentialHeaders = make([]managementquery.CredentialHeader, 0, len(decoded.Transport.CredentialHeaders))
		for _, binding := range decoded.Transport.CredentialHeaders {
			connection.CredentialHeaders = append(connection.CredentialHeaders, managementquery.CredentialHeader{
				Name: binding.Name, Scheme: binding.Scheme, CredentialRef: binding.Reference.String(),
			})
		}
	case mcpplugin.TransportStdio:
		connection.Transport = "stdio"
		connection.Executable = decoded.Transport.Executable
		connection.Args = append([]string(nil), decoded.Transport.Args...)
		connection.Directory = decoded.Transport.Directory
		connection.CredentialEnvironment = make([]managementquery.CredentialEnvironment, 0, len(decoded.Transport.CredentialEnvironment))
		for _, binding := range decoded.Transport.CredentialEnvironment {
			connection.CredentialEnvironment = append(connection.CredentialEnvironment, managementquery.CredentialEnvironment{
				Name: binding.Name, CredentialRef: binding.Reference.String(),
			})
		}
	}
	return view
}
