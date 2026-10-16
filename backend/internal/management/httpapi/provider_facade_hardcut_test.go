package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func TestPrivateProviderFacadesCannotBeCreatedThroughHTTP(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	options := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/provider-plugins", nil, http.StatusOK)
	items, ok := options["plugins"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("Provider picker options %#v", options)
	}
	seen := map[string]bool{}
	for _, v := range items {
		plugin, ok := v.(map[string]any)
		if !ok {
			t.Fatal("malformed Provider picker entry")
		}
		seen[plugin["pluginId"].(string)] = true
	}
	for _, id := range []string{"io.mender.provider.mcp", "io.mender.provider.openapi", "io.mender.provider.model-tools"} {
		if !seen[id] {
			t.Fatalf("missing independent create option %s", id)
		}
	}
	for _, id := range []string{"io.mender.provider.agent-delegation", "io.mender.provider.workspace", "io.mender.provider.sandbox-jobs"} {
		if seen[id] {
			t.Fatalf("managed facade offered to user: %s", id)
		}
		requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/providers", map[string]any{
			"namespace": "forbidden", "displayName": "Forbidden", "pluginId": id,
			"config": map[string]any{"sandboxId": "sbx_0123456789abcdef0123456789abcdef", "agentId": "agent_0123456789abcdef0123456789abcdef"},
		}, http.StatusForbidden)
	}
	providers, err := runtime.Catalog.ListProviders.Execute(ctx)
	if err != nil || len(providers) != 0 {
		t.Fatalf("denied HTTP creations mutated Providers: %d %v", len(providers), err)
	}
}

func TestManagedProviderSyncIsOnlyAllowedFromSandboxToolSuiteOwner(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	caller, err := governancedomain.NewContext(
		governancedomain.OriginInternal, runtime.OwnerPrincipal(), governancedomain.CatalogScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := runtime.Sandboxes.CreateLocalSandbox(ctx, bootstrap.CreateLocalSandboxRequest{
		Name: "HTTP facade test", Root: t.TempDir(), Isolation: "host",
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := runtime.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), caller)
	if err != nil || suite.Status != "ready" || len(suite.Tools) != 4 {
		t.Fatalf("owner preparation: %+v, %v", suite, err)
	}
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	sources := requestJSON(t, server.Client(), http.MethodGet,
		server.URL+"/api/v1/providers", nil, http.StatusOK)
	providerItems, ok := sources["providers"].([]any)
	if !ok || len(providerItems) != 1 {
		t.Fatalf("managed Provider sources = %#v", sources)
	}
	item, ok := providerItems[0].(map[string]any)
	if !ok || item["providerId"] != suite.ProviderID ||
		item["toolSourceKind"] != "sandbox" ||
		item["toolSourceId"] != sandbox.ID().String() ||
		item["toolSourceName"] != "HTTP facade test" {
		t.Fatalf("Tool source attribution did not use resource ownership binding: %#v", item)
	}
	requestJSON(t, server.Client(), http.MethodPost,
		server.URL+"/api/v1/providers/"+suite.ProviderID+"/sync", nil, http.StatusConflict)
	after, err := runtime.SandboxWorkspaceTools(ctx, sandbox.ID(), caller)
	if err != nil || after.ProviderID != suite.ProviderID || after.Status != "ready" ||
		len(after.Tools) != len(suite.Tools) {
		t.Fatalf("generic sync altered managed tools: %+v %v", after, err)
	}
	for i := range after.Tools {
		if after.Tools[i].ToolID != suite.Tools[i].ToolID {
			t.Fatalf("ToolID changed after denied generic sync: %s != %s",
				after.Tools[i].ToolID, suite.Tools[i].ToolID)
		}
	}
}
