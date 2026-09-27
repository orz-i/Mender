package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

func TestRuntimeImportsLegacyMCPProviderConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "mender.db")
	providerID := testProviderPluginProviderID(
		t,
		"prov_00000000000000000000000000000010",
	)

	legacy, err := mcpconfigstore.Open(ctx, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Save(ctx, mcpconfigstore.Config{
		ProviderID: providerID,
		Transport:  mcpconfigstore.TransportHTTP,
		Endpoint:   "http://127.0.0.1:9876/mcp",
	}); err != nil {
		_ = legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:   databasePath,
		DefaultTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	mapped, err := runtime.providerConfigs.GetProviderConfig(ctx, providerID)
	if err != nil {
		t.Fatalf("GetProviderConfig() error = %v", err)
	}
	if mapped.PluginID != mcpplugin.ID {
		t.Fatalf("PluginID = %s, want %s", mapped.PluginID, mcpplugin.ID)
	}
	config, err := mcpplugin.DecodeConfig(mapped.Config)
	if err != nil {
		t.Fatal(err)
	}
	if config.Transport.Kind != mcpplugin.TransportStreamableHTTP ||
		config.Transport.Endpoint != "http://127.0.0.1:9876/mcp" {
		t.Fatalf("imported MCP config = %+v", config)
	}
}

func TestMCPCompatibilityLifecycleKeepsGenericConfigInSync(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:   filepath.Join(t.TempDir(), "mender.db"),
		DefaultTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	provider, err := runtime.RegisterMCPProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "compat-provider",
			DisplayName: "Compat Provider",
		},
		mcpconfigstore.Config{
			Transport:  mcpconfigstore.TransportStdio,
			Executable: "server-v1",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	generic, err := runtime.providerConfigs.GetProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	config, err := mcpplugin.DecodeConfig(generic.Config)
	if err != nil {
		t.Fatal(err)
	}
	if config.Transport.Executable != "server-v1" {
		t.Fatalf("registered executable = %q", config.Transport.Executable)
	}

	_, err = runtime.UpdateMCPProvider(
		ctx,
		provider.ID(),
		catalogapplication.UpdateProviderRequest{
			DisplayName: "Compat Provider Updated",
		},
		mcpconfigstore.Config{
			Transport:  mcpconfigstore.TransportStdio,
			Executable: "server-v2",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	generic, err = runtime.providerConfigs.GetProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	config, err = mcpplugin.DecodeConfig(generic.Config)
	if err != nil {
		t.Fatal(err)
	}
	if config.Transport.Executable != "server-v2" {
		t.Fatalf("updated executable = %q", config.Transport.Executable)
	}

	if err := runtime.DeleteMCPProvider(ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.providerConfigs.GetProviderConfig(
		ctx,
		provider.ID(),
	); !errors.Is(err, providerconfigstore.ErrNotFound) {
		t.Fatalf("generic config after delete error = %v", err)
	}
	if _, err := runtime.mcpConfigs.Get(
		ctx,
		provider.ID(),
	); !errors.Is(err, mcpconfigstore.ErrNotFound) {
		t.Fatalf("compatibility config after delete error = %v", err)
	}
}

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
