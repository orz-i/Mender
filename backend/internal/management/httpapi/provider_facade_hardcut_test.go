package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
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
