package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestUnifiedPluginCatalogueManagementContract(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	server := httptest.NewServer(New(r))
	defer server.Close()
	result := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/plugins", nil, http.StatusOK)
	items, ok := result["plugins"].([]any)
	if !ok || len(items) < 6 {
		t.Fatalf("catalogue missing builtin types: %+v", result)
	}
	kinds := map[string]bool{}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("invalid catalogue entry")
		}
		kind, ok := item["kind"].(string)
		if !ok {
			t.Fatal("missing typed extension kind")
		}
		kinds[kind] = true
	}
	for _, kind := range []string{"provider.tools", "sandbox.backend", "tunnel.transport"} {
		if !kinds[kind] {
			t.Fatalf("missing extension kind %s", kind)
		}
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/plugin-installations/pinst_missing/disable", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/external", map[string]any{
		"name": "Denied", "installationId": "invalid", "config": map[string]any{},
	}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/tunnels/external", map[string]any{
		"displayName": "Denied", "installationId": "invalid", "externalTunnelId": "test", "runtimeCredentialRef": "credential://demo/key", // #nosec G101 -- non-secret test-only Credential Reference.
	}, http.StatusBadRequest)
	// No file-system install endpoint exists on the HTTP management plane.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/plugin-installations", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("public path installation unexpectedly exposed: %d", response.StatusCode)
	}
}
