package bootstrap

import (
	"context"
	"encoding/json"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func testCatalogGovernanceContext(
	t *testing.T,
	origin governancedomain.Origin,
	label string,
) governancedomain.Context {
	t.Helper()
	ctx, err := governancedomain.NewContext(
		origin,
		testPrincipal(t, label),
		governancedomain.CatalogScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func testPrincipal(t *testing.T, label string) governancedomain.Principal {
	t.Helper()
	if label == "" {
		label = "default"
	}
	principal, err := governancedomain.NewPrincipal(
		governancedomain.PrincipalService,
		"test:"+label,
	)
	if err != nil {
		t.Fatal(err)
	}
	return principal
}

func registerConfiguredProviderTest(
	t *testing.T,
	runtime *Runtime,
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	t.Helper()
	caller := testCatalogGovernanceContext(t, governancedomain.OriginInternal, "register")
	if isManagedProviderFacade(pluginID) {
		// Historical fixture only: construct a pre-ownership Provider so
		// adoption/conflict tests can prove that identities remain unchanged.
		// Production HTTP/CLI and exported Runtime creation stay denied.
		return runtime.registerConfiguredProvider(ctx, request, pluginID, rawConfig, caller, true)
	}
	return runtime.RegisterConfiguredProvider(ctx, request, pluginID, rawConfig, caller)
}

// configureAttachedMCPProviderTest records a valid, verified standalone
// Provider identity for an already attached in-memory upstream MCP fixture.
// It never dials the placeholder endpoint or starts a provider process.
func configureAttachedMCPProviderTest(
	t *testing.T, runtime *Runtime, ctx context.Context, providerID catalogdomain.ProviderID,
) {
	t.Helper()
	raw, err := json.Marshal(mcpplugin.Config{
		Transport: mcpplugin.TransportConfig{
			Kind: mcpplugin.TransportStreamableHTTP, Endpoint: "http://127.0.0.1:1/mcp",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpplugin.New().ValidateConfig(ctx, raw); err != nil {
		t.Fatalf("fixture MCP config: %v", err)
	}
	config, err := providerplugin.NewConfiguration(providerID, mcpplugin.ID, mcpplugin.ConfigVersion, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.providerConfigs.Save(ctx, config); err != nil {
		t.Fatalf("store verified fixture Provider configuration: %v", err)
	}
}

func registerInstalledProviderTest(
	t *testing.T,
	runtime *Runtime,
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	t.Helper()
	return runtime.RegisterInstalledProvider(
		ctx,
		request,
		pluginID,
		installationID,
		rawConfig,
		testCatalogGovernanceContext(t, governancedomain.OriginInternal, "register-installed"),
	)
}

func updateConfiguredProviderTest(
	t *testing.T,
	runtime *Runtime,
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	request catalogapplication.UpdateProviderRequest,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	t.Helper()
	return runtime.UpdateConfiguredProvider(
		ctx,
		providerID,
		request,
		rawConfig,
		testCatalogGovernanceContext(t, governancedomain.OriginInternal, "update"),
	)
}

func syncConfiguredProviderTest(
	t *testing.T,
	runtime *Runtime,
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (catalogapplication.SyncProviderToolsResult, error) {
	t.Helper()
	governance := testCatalogGovernanceContext(t, governancedomain.OriginInternal, "sync")
	config, err := runtime.ProviderConfig(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	if isManagedProviderFacade(config.PluginID) {
		return runtime.syncConfiguredProviderForOwner(ctx, providerID, governance, true)
	}
	return runtime.SyncConfiguredProvider(ctx, providerID, governance)
}

func rebindProviderInstallationTest(
	t *testing.T,
	runtime *Runtime,
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogapplication.SyncProviderToolsResult, error) {
	t.Helper()
	return runtime.RebindProviderInstallation(
		ctx,
		providerID,
		installationID,
		rawConfig,
		testCatalogGovernanceContext(t, governancedomain.OriginInternal, "rebind"),
	)
}

func startMCPHTTPServerTest(
	t *testing.T,
	runtime *Runtime,
	ctx context.Context,
	slug profiledomain.Slug,
	address string,
) (MCPServerInstance, error) {
	t.Helper()
	return runtime.StartMCPHTTPServer(
		ctx,
		slug,
		address,
		testCatalogGovernanceContext(t, governancedomain.OriginInternal, "server-start"),
	)
}

func restartMCPServerTest(
	t *testing.T,
	runtime *Runtime,
	ctx context.Context,
	serverID string,
) (MCPServerInstance, error) {
	t.Helper()
	return runtime.RestartMCPServer(
		ctx,
		serverID,
		testCatalogGovernanceContext(t, governancedomain.OriginInternal, "server-restart"),
	)
}

func activateConfiguredProviderTest(
	t *testing.T,
	runtime *Runtime,
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	t.Helper()
	return runtime.activateConfiguredProvider(
		ctx,
		providerID,
		testCatalogGovernanceContext(t, governancedomain.OriginInternal, "activate"),
	)
}
