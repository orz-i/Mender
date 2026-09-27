package bootstrap

import (
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
)

func TestLegacyMCPProviderConfigurationMapsHTTP(t *testing.T) {
	t.Parallel()

	providerID := testProviderPluginProviderID(t, "prov_00000000000000000000000000000011")
	ref, err := credentialdomain.NewReference("demo", "token")
	if err != nil {
		t.Fatal(err)
	}
	legacy := mcpconfigstore.Config{
		ProviderID: providerID,
		Transport:  mcpconfigstore.TransportHTTP,
		Endpoint:   "https://example.test/mcp",
		Headers: map[string]string{
			"X-Client": "mender",
		},
		CredentialHeaders: []mcpconfigstore.CredentialHeader{
			{Name: "Authorization", Scheme: "Bearer", Reference: ref},
		},
	}

	mapped, err := legacyMCPProviderConfiguration(legacy)
	if err != nil {
		t.Fatalf("legacyMCPProviderConfiguration() error = %v", err)
	}
	if mapped.ProviderID != providerID {
		t.Fatalf("ProviderID = %s, want %s", mapped.ProviderID, providerID)
	}
	if mapped.PluginID != mcpplugin.ID {
		t.Fatalf("PluginID = %s, want %s", mapped.PluginID, mcpplugin.ID)
	}
	if mapped.ConfigVersion != mcpplugin.ConfigVersion {
		t.Fatalf(
			"ConfigVersion = %d, want %d",
			mapped.ConfigVersion,
			mcpplugin.ConfigVersion,
		)
	}
	if mapped.Revision == "" {
		t.Fatal("Revision is empty")
	}

	config, err := mcpplugin.DecodeConfig(mapped.Config)
	if err != nil {
		t.Fatalf("DecodeConfig() error = %v", err)
	}
	if config.Transport.Kind != mcpplugin.TransportStreamableHTTP {
		t.Fatalf("transport kind = %q", config.Transport.Kind)
	}
	if config.Transport.Endpoint != legacy.Endpoint {
		t.Fatalf("endpoint = %q, want %q", config.Transport.Endpoint, legacy.Endpoint)
	}
	if len(config.Transport.CredentialHeaders) != 1 ||
		config.Transport.CredentialHeaders[0].Reference != ref {
		t.Fatalf("credential headers = %+v", config.Transport.CredentialHeaders)
	}

	again, err := legacyMCPProviderConfiguration(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if again.Revision != mapped.Revision {
		t.Fatalf(
			"revision changed for identical config: %q != %q",
			again.Revision,
			mapped.Revision,
		)
	}
}

func TestLegacyMCPProviderConfigurationMapsStdio(t *testing.T) {
	t.Parallel()

	providerID := testProviderPluginProviderID(t, "prov_00000000000000000000000000000012")
	ref, err := credentialdomain.NewReference("demo", "token")
	if err != nil {
		t.Fatal(err)
	}
	legacy := mcpconfigstore.Config{
		ProviderID: providerID,
		Transport:  mcpconfigstore.TransportStdio,
		Executable: "mcp-server",
		Args:       []string{"--stdio"},
		Directory:  "/tmp",
		CredentialEnvironment: []mcpconfigstore.CredentialEnvironment{
			{Name: "API_TOKEN", Reference: ref},
		},
	}

	mapped, err := legacyMCPProviderConfiguration(legacy)
	if err != nil {
		t.Fatalf("legacyMCPProviderConfiguration() error = %v", err)
	}
	config, err := mcpplugin.DecodeConfig(mapped.Config)
	if err != nil {
		t.Fatalf("DecodeConfig() error = %v", err)
	}
	if config.Transport.Kind != mcpplugin.TransportStdio {
		t.Fatalf("transport kind = %q", config.Transport.Kind)
	}
	if config.Transport.Executable != legacy.Executable {
		t.Fatalf(
			"executable = %q, want %q",
			config.Transport.Executable,
			legacy.Executable,
		)
	}
	if len(config.Transport.CredentialEnvironment) != 1 ||
		config.Transport.CredentialEnvironment[0].Reference != ref {
		t.Fatalf(
			"credential environment = %+v",
			config.Transport.CredentialEnvironment,
		)
	}
}

func testProviderPluginProviderID(
	t *testing.T,
	value string,
) catalogdomain.ProviderID {
	t.Helper()
	id, err := catalogdomain.ParseProviderID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
