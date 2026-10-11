package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestHTTPProfileSandboxGrantLifecycle(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "http-grants.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	sandbox := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local",
		map[string]any{"name": "Grant fixture", "root": t.TempDir(), "isolation": "host"}, http.StatusCreated)
	profile := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/profiles",
		map[string]any{"slug": "grant-fixture", "displayName": "Grant fixture", "mode": "hybrid"}, http.StatusCreated)
	base := server.URL + "/api/v1/sandboxes/" + sandbox["sandboxId"].(string)
	path := base + "/profile-grants/" + profile["profileId"].(string)
	got := requestJSON(t, server.Client(), http.MethodGet, base+"/profile-grants", nil, http.StatusOK)
	if len(got["grants"].([]any)) != 0 {
		t.Fatal("implicit grant")
	}
	requestJSON(t, server.Client(), http.MethodPut, path, map[string]any{"capabilities": []string{}}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPut, path, map[string]any{"capabilities": []string{"file.read", "invalid"}}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPut, base+"/profile-grants/prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", map[string]any{"capabilities": []string{"file.read"}}, http.StatusNotFound)
	got = requestJSON(t, server.Client(), http.MethodPut, path, map[string]any{"capabilities": []string{"file.read", "file.search"}}, http.StatusOK)
	if len(got["grants"].([]any)) != 1 {
		t.Fatalf("grants %+v", got)
	}
	events := requestJSON(t, server.Client(), http.MethodGet, base+"/profile-grant-events", nil, http.StatusOK)
	if len(events["events"].([]any)) != 2 {
		t.Fatalf("grant audit %+v", events)
	}
	first := events["events"].([]any)[0].(map[string]any)
	for _, k := range []string{"principalRef", "secret", "arguments", "ownerRef"} {
		if _, ok := first[k]; ok {
			t.Fatalf("audit leaked %s", k)
		}
	}
	got = requestJSON(t, server.Client(), http.MethodDelete, path, nil, http.StatusOK)
	if len(got["grants"].([]any)) != 0 {
		t.Fatal("revoke failed")
	}
	events = requestJSON(t, server.Client(), http.MethodGet, base+"/profile-grant-events", nil, http.StatusOK)
	if len(events["events"].([]any)) != 4 {
		t.Fatalf("revoke audit %+v", events)
	}
}
