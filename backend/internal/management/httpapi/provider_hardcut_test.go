package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

// The old MCP HTTP/stdio URLs and flat Provider updates are removed rather
// than kept as aliases. A rejected legacy attempt must not create a Provider.
func TestHardCutRejectsLegacyProviderRoutesWithoutWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	for _, path := range []string{"/api/v1/providers/http", "/api/v1/providers/stdio"} {
		response, err := server.Client().Post(server.URL+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode < 400 {
			t.Fatalf("retired %s accepted with status %d", path, response.StatusCode)
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, server.URL+"/api/v1/providers/prov_00000000000000000000000000000001", strings.NewReader(`{"transport":"http","displayName":"Old","endpoint":"https://example.test"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("flat Provider PUT status=%d, want 400", response.StatusCode)
	}

	providers, err := runtime.Catalog.ListProviders.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 0 {
		t.Fatalf("retired routes created %d Providers", len(providers))
	}
}
