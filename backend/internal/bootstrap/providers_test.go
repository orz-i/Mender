package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

func TestRuntimeExposesBuiltInProviderPluginDescriptors(t *testing.T) {
	t.Parallel()

	runtime := newProviderTestRuntime(t)
	descriptors := runtime.ProviderPluginDescriptors()
	if len(descriptors) != 1 {
		t.Fatalf("ProviderPluginDescriptors() len = %d, want 1", len(descriptors))
	}
	if descriptors[0].ID != mcpplugin.ID {
		t.Fatalf(
			"ProviderPluginDescriptors()[0].ID = %s, want %s",
			descriptors[0].ID,
			mcpplugin.ID,
		)
	}
	if descriptors[0].ConfigVersion != mcpplugin.ConfigVersion {
		t.Fatalf(
			"ProviderPluginDescriptors()[0].ConfigVersion = %d, want %d",
			descriptors[0].ConfigVersion,
			mcpplugin.ConfigVersion,
		)
	}
}

func TestRegisterAndUpdateConfiguredProviderPreservePluginIdentity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newProviderTestRuntime(t)

	stdioConfig := mustProviderConfigJSON(t, mcpplugin.Config{
		Transport: mcpplugin.TransportConfig{
			Kind:       mcpplugin.TransportStdio,
			Executable: "mcp-server",
			Args:       []string{"--stdio"},
		},
	})
	provider, err := runtime.RegisterConfiguredProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "generic",
			DisplayName: "Generic MCP",
		},
		mcpplugin.ID,
		stdioConfig,
	)
	if err != nil {
		t.Fatalf("RegisterConfiguredProvider() error = %v", err)
	}

	stored, err := runtime.ProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatalf("ProviderConfig() error = %v", err)
	}
	if stored.PluginID != mcpplugin.ID {
		t.Fatalf("PluginID = %s, want %s", stored.PluginID, mcpplugin.ID)
	}
	firstRevision := stored.Revision

	httpConfig := mustProviderConfigJSON(t, mcpplugin.Config{
		Transport: mcpplugin.TransportConfig{
			Kind:     mcpplugin.TransportStreamableHTTP,
			Endpoint: "https://example.test/mcp",
		},
	})
	updated, err := runtime.UpdateConfiguredProvider(
		ctx,
		provider.ID(),
		catalogapplication.UpdateProviderRequest{
			DisplayName: "Generic MCP edited",
			Description: "same plugin, different transport",
		},
		httpConfig,
	)
	if err != nil {
		t.Fatalf("UpdateConfiguredProvider() error = %v", err)
	}
	if updated.ID() != provider.ID() {
		t.Fatalf("ProviderID changed: %s -> %s", provider.ID(), updated.ID())
	}

	stored, err = runtime.ProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatalf("ProviderConfig(after update) error = %v", err)
	}
	if stored.PluginID != mcpplugin.ID {
		t.Fatalf("PluginID after update = %s, want %s", stored.PluginID, mcpplugin.ID)
	}
	if stored.Revision == firstRevision {
		t.Fatal("provider config revision did not change")
	}
	decoded, err := mcpplugin.DecodeConfig(stored.Config)
	if err != nil {
		t.Fatalf("DecodeConfig() error = %v", err)
	}
	if decoded.Transport.Kind != mcpplugin.TransportStreamableHTTP {
		t.Fatalf(
			"transport kind = %q, want %q",
			decoded.Transport.Kind,
			mcpplugin.TransportStreamableHTTP,
		)
	}
}

func TestRegisterConfiguredProviderRejectsUnknownPlugin(t *testing.T) {
	t.Parallel()

	runtime := newProviderTestRuntime(t)
	unknown, err := providerplugin.ParseID("com.example.provider.unknown")
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RegisterConfiguredProvider(
		context.Background(),
		catalogapplication.RegisterProviderRequest{
			Namespace:   "unknown",
			DisplayName: "Unknown",
		},
		unknown,
		json.RawMessage(`{"value":true}`),
	)
	if !errors.Is(err, providerplugin.ErrNotFound) {
		t.Fatalf("RegisterConfiguredProvider() error = %v, want ErrNotFound", err)
	}

	providers, listErr := runtime.Catalog.ListProviders.Execute(context.Background())
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(providers) != 0 {
		t.Fatalf("providers = %+v, want none after rejected plugin", providers)
	}
}

func TestDeleteConfiguredProviderRemovesGenericState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newProviderTestRuntime(t)
	raw := mustProviderConfigJSON(t, mcpplugin.Config{
		Transport: mcpplugin.TransportConfig{
			Kind:       mcpplugin.TransportStdio,
			Executable: "mcp-server",
		},
	})
	provider, err := runtime.RegisterConfiguredProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "delete-me",
			DisplayName: "Delete Me",
		},
		mcpplugin.ID,
		raw,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := runtime.DeleteConfiguredProvider(ctx, provider.ID()); err != nil {
		t.Fatalf("DeleteConfiguredProvider() error = %v", err)
	}
	if _, err := runtime.ProviderConfig(ctx, provider.ID()); !errors.Is(err, providerconfigstore.ErrNotFound) {
		t.Fatalf("ProviderConfig(after delete) error = %v, want ErrNotFound", err)
	}
	if _, err := runtime.Catalog.GetProvider.Execute(ctx, provider.ID()); !errors.Is(
		err,
		catalogapplication.ErrProviderNotFound,
	) {
		t.Fatalf("GetProvider(after delete) error = %v, want ErrProviderNotFound", err)
	}
}

func newProviderTestRuntime(t *testing.T) *Runtime {
	t.Helper()

	runtime, err := NewRuntime(context.Background(), RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	return runtime
}

func mustProviderConfigJSON(t *testing.T, config mcpplugin.Config) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestConfiguredProviderRuntimeReusesAndReplacesPluginInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newProviderTestRuntime(t)
	plugin := &runtimeTestProviderPlugin{
		descriptor: providerplugin.Descriptor{
			ID:            providerplugin.ID("io.mender.provider.runtime-test"),
			Version:       "1.0.0",
			DisplayName:   "Runtime Test",
			Description:   "Test Provider plugin",
			ConfigVersion: 1,
		},
	}
	if err := runtime.providerPlugins.Register(plugin); err != nil {
		t.Fatalf("Register(test plugin) error = %v", err)
	}

	provider, err := runtime.RegisterConfiguredProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "runtime-test",
			DisplayName: "Runtime Test",
		},
		plugin.descriptor.ID,
		json.RawMessage(`{"revision":1}`),
	)
	if err != nil {
		t.Fatalf("RegisterConfiguredProvider() error = %v", err)
	}
	syncResult, err := runtime.SyncConfiguredProvider(ctx, provider.ID())
	if err != nil {
		t.Fatalf("SyncConfiguredProvider() error = %v", err)
	}
	if syncResult.Total != 1 {
		t.Fatalf("SyncConfiguredProvider().Total = %d, want 1", syncResult.Total)
	}
	if got := plugin.counts(); got.opens != 1 || got.closes != 0 {
		t.Fatalf("plugin counts after sync = %+v, want opens=1 closes=0", got)
	}

	search, err := runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{
			Query:      "echo",
			Limit:      10,
			ProviderID: providerIDPtr(provider.ID()),
		},
	)
	if err != nil {
		t.Fatalf("SearchTools.Execute() error = %v", err)
	}
	if len(search) != 1 {
		t.Fatalf("SearchTools.Execute() = %+v, want one Tool", search)
	}

	invocationContext, err := invocationdomain.NewInvocationContext(
		invocationdomain.OriginInternal,
		"provider-plugin-runtime-test",
		invocationdomain.CatalogExecutionScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"first", "second"} {
		response, callErr := runtime.CallConfiguredTool(
			ctx,
			search[0].ToolID,
			[]byte(`{"value":"`+value+`"}`),
			invocationContext,
		)
		if callErr != nil {
			t.Fatalf("CallConfiguredTool(%s) error = %v", value, callErr)
		}
		if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf(
				"CallConfiguredTool(%s) outcome = %s, want complete",
				value,
				response.Outcome.Kind(),
			)
		}
	}
	if got := plugin.counts(); got.opens != 1 || got.closes != 0 {
		t.Fatalf(
			"plugin counts after repeated calls = %+v, want opens=1 closes=0",
			got,
		)
	}

	if _, err := runtime.UpdateConfiguredProvider(
		ctx,
		provider.ID(),
		catalogapplication.UpdateProviderRequest{
			DisplayName: "Runtime Test Updated",
		},
		json.RawMessage(`{"revision":2}`),
	); err != nil {
		t.Fatalf("UpdateConfiguredProvider() error = %v", err)
	}

	if _, err := runtime.CallConfiguredTool(
		ctx,
		search[0].ToolID,
		[]byte(`{"value":"third"}`),
		invocationContext,
	); err != nil {
		t.Fatalf("CallConfiguredTool(after revision) error = %v", err)
	}
	if got := plugin.counts(); got.opens != 2 || got.closes != 1 {
		t.Fatalf(
			"plugin counts after revision change = %+v, want opens=2 closes=1",
			got,
		)
	}

	if err := runtime.DeleteConfiguredProvider(ctx, provider.ID()); err != nil {
		t.Fatalf("DeleteConfiguredProvider() error = %v", err)
	}
	if got := plugin.counts(); got.opens != 2 || got.closes != 2 {
		t.Fatalf(
			"plugin counts after delete = %+v, want opens=2 closes=2",
			got,
		)
	}
}

func providerIDPtr(id catalogdomain.ProviderID) *catalogdomain.ProviderID {
	return &id
}

type runtimeTestProviderPlugin struct {
	descriptor providerplugin.Descriptor

	mu     sync.Mutex
	opens  int
	closes int
}

func (p *runtimeTestProviderPlugin) Descriptor() providerplugin.Descriptor {
	return p.descriptor
}

func (*runtimeTestProviderPlugin) ValidateConfig(
	_ context.Context,
	raw json.RawMessage,
) error {
	if !json.Valid(raw) {
		return errors.New("invalid JSON")
	}
	return nil
}

func (*runtimeTestProviderPlugin) InspectConfig(
	_ context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	if !json.Valid(raw) {
		return providerplugin.ConfigMetadata{}, errors.New("invalid JSON")
	}
	return providerplugin.NewConfigMetadata(nil)
}

func (p *runtimeTestProviderPlugin) Open(
	_ context.Context,
	request providerplugin.OpenRequest,
) (providerplugin.Instance, error) {
	if request.ConfigVersion != p.descriptor.ConfigVersion {
		return nil, errors.New("unsupported config version")
	}
	p.mu.Lock()
	p.opens++
	p.mu.Unlock()
	return &runtimeTestProviderInstance{owner: p}, nil
}

func (p *runtimeTestProviderPlugin) counts() struct {
	opens  int
	closes int
} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return struct {
		opens  int
		closes int
	}{opens: p.opens, closes: p.closes}
}

type runtimeTestProviderInstance struct {
	owner *runtimeTestProviderPlugin
	once  sync.Once
}

func (*runtimeTestProviderInstance) DiscoverTools(
	_ context.Context,
	_ catalogdomain.Provider,
) ([]catalogapplication.DiscoveredTool, error) {
	return []catalogapplication.DiscoveredTool{
		{
			SourceKey:   "echo",
			Name:        "echo",
			Title:       "Echo",
			Description: "Echo JSON input",
			InputSchema: []byte(`{"type":"object"}`),
		},
	}, nil
}

func (*runtimeTestProviderInstance) Execute(
	_ context.Context,
	request invocationapplication.ExecutionRequest,
	_ invocationapplication.CredentialResolver,
	_ invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	return invocationdomain.Complete(request.Arguments)
}

func (i *runtimeTestProviderInstance) Close() error {
	i.once.Do(func() {
		i.owner.mu.Lock()
		i.owner.closes++
		i.owner.mu.Unlock()
	})
	return nil
}
