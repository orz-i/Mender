package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	openapiplugin "github.com/orz-i/Mender/backend/internal/openapi/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
)

func TestRuntimeExposesBuiltInProviderPluginDescriptors(t *testing.T) {
	t.Parallel()

	runtime := newProviderTestRuntime(t)
	descriptors := runtime.ProviderPluginDescriptors()
	if len(descriptors) != 4 {
		t.Fatalf("ProviderPluginDescriptors() len = %d, want 4", len(descriptors))
	}
	wantIDs := []providerplugin.ID{
		"io.mender.provider.mcp",
		"io.mender.provider.model-tools",
		"io.mender.provider.openapi",
		"io.mender.provider.workspace",
	}
	for index, descriptor := range descriptors {
		if descriptor.ID != wantIDs[index] {
			t.Fatalf(
				"ProviderPluginDescriptors()[%d].ID = %s, want %s",
				index,
				descriptor.ID,
				wantIDs[index],
			)
		}
		if descriptor.ConfigVersion != 1 {
			t.Fatalf(
				"ProviderPluginDescriptors()[%d].ConfigVersion = %d, want 1",
				index,
				descriptor.ConfigVersion,
			)
		}
	}

	available, err := runtime.ListAvailableProviderPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListAvailableProviderPlugins() error = %v", err)
	}
	if len(available) != 4 {
		t.Fatalf("ListAvailableProviderPlugins() len = %d, want 4", len(available))
	}
	for _, item := range available {
		if item.SourceKind != ProviderPluginSourceBuiltIn ||
			item.InstallationID != nil ||
			!item.Enabled {
			t.Fatalf("built-in available plugin = %+v", item)
		}
	}
}

func TestProviderPluginDescriptorForConfigUsesPersistedExternalSnapshotOffline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newProviderTestRuntime(t)
	installationID, err := providerplugin.ParseInstallationID(
		"pinst_00000000000000000000000000000097",
	)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID("dev.example.offline")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := providerplugin.Descriptor{
		ID:              pluginID,
		Version:         "2.3.4",
		DisplayName:     "Offline Plugin",
		Description:     "Persisted descriptor fixture",
		ConfigVersion:   3,
		ProtocolVersion: 1,
		ConfigSchema:    json.RawMessage(`{"type":"object"}`),
	}
	installation, err := installationdomain.NewInstallation(
		installationID,
		pluginID,
		"2.3.4",
		installationdomain.SourceLocal,
		"/tmp/mender-offline-provider-fixture",
		"sha256:"+strings.Repeat("7", 64),
		[]byte(`{
			"manifestVersion": 1,
			"pluginId": "dev.example.offline",
			"version": "2.3.4",
			"protocolVersions": [1],
			"displayName": "Offline Plugin",
			"description": "Persisted descriptor fixture",
			"entrypoints": {"linux/amd64": "bin/plugin"}
		}`),
		installationdomain.StatusDisabled,
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	installation, err = installation.WithVerifiedDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.pluginInstallations.Create(ctx, installation); err != nil {
		t.Fatal(err)
	}

	providerID, err := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000097",
	)
	if err != nil {
		t.Fatal(err)
	}
	config, err := providerplugin.NewInstalledConfiguration(
		providerID,
		pluginID,
		installationID,
		3,
		json.RawMessage(`{"name":"offline"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := runtime.ProviderPluginDescriptorForConfig(ctx, config)
	if err != nil {
		t.Fatalf("ProviderPluginDescriptorForConfig() error = %v", err)
	}
	if !ok ||
		got.ID != descriptor.ID ||
		got.Version != "2.3.4" ||
		got.DisplayName != "Offline Plugin" {
		t.Fatalf("descriptor = %+v present=%v", got, ok)
	}
	if runtime.externalProcesses.Running(installationID) {
		t.Fatal("offline descriptor lookup started external process")
	}
}

func TestListAvailableProviderPluginsIsolatesBrokenLegacyInstallation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newProviderTestRuntime(t)
	installationID, err := providerplugin.ParseInstallationID(
		"pinst_00000000000000000000000000000099",
	)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID("dev.example.broken")
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON := []byte(`{
		"manifestVersion": 1,
		"pluginId": "dev.example.broken",
		"version": "1.0.0",
		"protocolVersions": [1],
		"displayName": "Broken",
		"description": "Missing managed package fixture",
		"entrypoints": {"linux/amd64": "bin/plugin"}
	}`)
	installation, err := installationdomain.NewInstallation(
		installationID,
		pluginID,
		"1.0.0",
		installationdomain.SourceLocal,
		"/tmp/mender-broken-provider-fixture",
		"sha256:"+strings.Repeat("9", 64),
		manifestJSON,
		installationdomain.StatusEnabled,
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.pluginInstallations.Create(ctx, installation); err != nil {
		t.Fatal(err)
	}

	catalog, err := runtime.ListAvailableProviderPluginCatalog(ctx)
	if err != nil {
		t.Fatalf("ListAvailableProviderPluginCatalog() error = %v", err)
	}
	if len(catalog.Plugins) != 4 {
		t.Fatalf("available plugins = %d, want 4 healthy built-ins", len(catalog.Plugins))
	}
	for _, item := range catalog.Plugins {
		if item.SourceKind != ProviderPluginSourceBuiltIn {
			t.Fatalf("unexpected unavailable external plugin in catalog: %+v", item)
		}
	}
	if len(catalog.Issues) != 1 ||
		catalog.Issues[0].InstallationID != installationID ||
		catalog.Issues[0].Code != providerPluginIssueResolveDescriptor {
		t.Fatalf("catalog issues = %+v", catalog.Issues)
	}
}

func TestBackfillProviderPluginDescriptorSurfacesPersistenceAndCleanupFailures(t *testing.T) {
	t.Parallel()

	installationID, err := providerplugin.ParseInstallationID(
		"pinst_00000000000000000000000000000098",
	)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID("dev.example.backfill")
	if err != nil {
		t.Fatal(err)
	}
	installation, err := installationdomain.NewInstallation(
		installationID,
		pluginID,
		"1.0.0",
		installationdomain.SourceLocal,
		"/tmp/mender-backfill-provider-fixture",
		"sha256:"+strings.Repeat("8", 64),
		[]byte(`{
			"manifestVersion": 1,
			"pluginId": "dev.example.backfill",
			"version": "1.0.0",
			"protocolVersions": [1],
			"displayName": "Backfill",
			"description": "Backfill fixture",
			"entrypoints": {"linux/amd64": "bin/plugin"}
		}`),
		installationdomain.StatusEnabled,
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := providerplugin.Descriptor{
		ID:              pluginID,
		Version:         "1.0.0",
		DisplayName:     "Backfill",
		ConfigVersion:   1,
		ProtocolVersion: 1,
	}

	t.Run("persistence failure still attempts cleanup", func(t *testing.T) {
		closer := &backfillProcessCloser{err: errors.New("cleanup failed")}
		_, verified, issues := backfillProviderPluginDescriptor(
			t.Context(),
			installation,
			backfillResolver{descriptor: descriptor},
			backfillDescriptorStore{err: errors.New("database is read-only")},
			closer,
		)
		if verified {
			t.Fatal("backfill unexpectedly reported verified descriptor")
		}
		if !closer.called {
			t.Fatal("backfill did not attempt process cleanup after persistence failure")
		}
		if len(issues) != 2 ||
			issues[0].Code != providerPluginIssuePersistDescriptor ||
			issues[1].Code != providerPluginIssueCloseProcess {
			t.Fatalf("issues = %+v", issues)
		}
	})

	t.Run("cleanup failure remains scoped after durable snapshot", func(t *testing.T) {
		closer := &backfillProcessCloser{err: errors.New("cleanup failed")}
		got, verified, issues := backfillProviderPluginDescriptor(
			t.Context(),
			installation,
			backfillResolver{descriptor: descriptor},
			backfillDescriptorStore{},
			closer,
		)
		if !verified || got.ID != descriptor.ID {
			t.Fatalf("backfill = descriptor %+v verified=%v", got, verified)
		}
		if len(issues) != 1 || issues[0].Code != providerPluginIssueCloseProcess {
			t.Fatalf("issues = %+v", issues)
		}
	})
}

type backfillResolver struct {
	descriptor providerplugin.Descriptor
	err        error
}

func (r backfillResolver) ResolveInstallation(
	context.Context,
	providerplugin.InstallationID,
) (providerplugin.Resolution, error) {
	if r.err != nil {
		return providerplugin.Resolution{}, r.err
	}
	return providerplugin.Resolution{Descriptor: r.descriptor}, nil
}

type backfillDescriptorStore struct {
	err error
}

func (s backfillDescriptorStore) SetVerifiedDescriptor(
	context.Context,
	providerplugin.InstallationID,
	providerplugin.Descriptor,
) error {
	return s.err
}

type backfillProcessCloser struct {
	called bool
	err    error
}

func (c *backfillProcessCloser) CloseInstallation(providerplugin.InstallationID) error {
	c.called = true
	return c.err
}

func TestOpenAPIProviderRuntimeEndToEndAndRefreshPreservesToolID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var sourceRevision atomic.Int32
	var apiCalls atomic.Int32
	sourceRevision.Store(1)

	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		revision := sourceRevision.Load()
		if revision >= 3 {
			spec := `openapi: 3.1.2
info:
  title: Demo
  version: "1"
servers:
  - url: ` + serverURL + `
paths: {}
`
			_, _ = w.Write([]byte(spec))
			return
		}
		summary := "Echo v1"
		if revision == 2 {
			summary = "Echo v2"
		}
		spec := `openapi: 3.1.2
info:
  title: Demo
  version: "1"
servers:
  - url: ` + serverURL + `
paths:
  /echo/{id}:
    get:
      operationId: echo
      summary: ` + summary + `
      parameters:
        - name: id
          in: path
          required: true
          schema: { type: string }
        - name: q
          in: query
          schema: { type: string }
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                required: [id, q]
                properties:
                  id: { type: string }
                  q: { type: string }
`
		_, _ = w.Write([]byte(spec))
	})
	mux.HandleFunc("/echo/", func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id": strings.TrimPrefix(r.URL.Path, "/echo/"),
			"q":  r.URL.Query().Get("q"),
		})
	})
	server := httptest.NewServer(mux)
	serverURL = server.URL
	t.Cleanup(server.Close)

	runtime := newProviderTestRuntime(t)
	rawConfig, err := json.Marshal(openapiplugin.Config{
		Source: openapiplugin.SourceConfig{
			Kind: openapiplugin.SourceURL,
			URL:  server.URL + "/openapi.yaml",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := runtime.RegisterConfiguredProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "openapi-demo",
			DisplayName: "OpenAPI Demo",
		},
		openapiplugin.ID,
		rawConfig,
	)
	if err != nil {
		t.Fatalf("RegisterConfiguredProvider() error = %v", err)
	}
	syncResult, err := runtime.SyncConfiguredProvider(ctx, provider.ID())
	if err != nil {
		t.Fatalf("SyncConfiguredProvider() error = %v", err)
	}
	if syncResult.Created != 1 || syncResult.Total != 1 {
		t.Fatalf("initial sync = %+v", syncResult)
	}

	search, err := runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{
			Query:      "echo",
			Limit:      10,
			ProviderID: providerIDPtr(provider.ID()),
		},
	)
	if err != nil || len(search) != 1 {
		t.Fatalf("search = %+v err=%v", search, err)
	}
	toolID := search[0].ToolID
	if search[0].Title != "Echo v1" {
		t.Fatalf("initial title = %q", search[0].Title)
	}

	callContext, err := governancedomain.NewContext(
		governancedomain.OriginInternal,
		testPrincipal(t, "openapi-runtime-test"),
		governancedomain.CatalogScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := runtime.CallConfiguredTool(
		ctx,
		toolID,
		[]byte(`{"path":{"id":"abc"},"query":{"q":"hello"}}`),
		callContext,
	)
	if err != nil {
		t.Fatalf("CallConfiguredTool() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("outcome = %#v", response.Outcome)
	}
	if apiCalls.Load() != 1 {
		t.Fatalf("API calls = %d, want 1", apiCalls.Load())
	}

	sourceRevision.Store(2)
	syncResult, err = runtime.SyncConfiguredProvider(ctx, provider.ID())
	if err != nil {
		t.Fatalf("refresh sync error = %v", err)
	}
	if syncResult.Updated != 1 || syncResult.Total != 1 {
		t.Fatalf("refresh sync = %+v", syncResult)
	}
	search, err = runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{
			Query:      "echo",
			Limit:      10,
			ProviderID: providerIDPtr(provider.ID()),
		},
	)
	if err != nil || len(search) != 1 {
		t.Fatalf("refresh search = %+v err=%v", search, err)
	}
	if search[0].ToolID != toolID {
		t.Fatalf("ToolID changed across SourceKey-stable refresh: %s -> %s", toolID, search[0].ToolID)
	}
	if search[0].Title != "Echo v2" {
		t.Fatalf("refreshed title = %q", search[0].Title)
	}

	sourceRevision.Store(3)
	syncResult, err = runtime.SyncConfiguredProvider(ctx, provider.ID())
	if err != nil {
		t.Fatalf("removal sync error = %v", err)
	}
	if syncResult.Removed != 1 || syncResult.Total != 0 {
		t.Fatalf("removal sync = %+v", syncResult)
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

	invocationContext, err := governancedomain.NewContext(
		governancedomain.OriginInternal,
		testPrincipal(t, "provider-plugin-runtime-test"),
		governancedomain.CatalogScope(),
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
