package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestHTTPWorkspaceToolTargetsReuseOneFacade(t *testing.T) {
	ctx := context.Background()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "workspace-targets.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	server := httptest.NewServer(New(r))
	defer server.Close()
	api := server.URL + "/api/v1"
	makeSandbox := func(name string) string {
		sandbox := requestJSON(t, server.Client(), http.MethodPost, api+"/sandboxes/local",
			map[string]any{"name": name, "root": t.TempDir(), "isolation": "host"}, http.StatusCreated)
		return sandbox["sandboxId"].(string)
	}
	source := makeSandbox("Source")
	alternate := makeSandbox("Alternate")
	suite := requestJSON(t, server.Client(), http.MethodPost, api+"/sandboxes/"+source+"/tools", nil, http.StatusOK)
	pid := suite["providerId"].(string)
	uri := api + "/tool-services/" + pid + "/workspaces"
	before := requestJSON(t, server.Client(), http.MethodGet, uri, nil, http.StatusOK)
	if len(before["workspaces"].([]any)) != 0 {
		t.Fatal("unexpected target before explicit bind")
	}
	targetPath := uri + "/review"
	requestJSON(t, server.Client(), http.MethodPut, targetPath, map[string]any{"sandboxId": alternate}, http.StatusNoContent)
	targets := requestJSON(t, server.Client(), http.MethodGet, uri, nil, http.StatusOK)
	if len(targets["workspaces"].([]any)) != 1 {
		t.Fatal("new target missing", targets)
	}
	requestJSON(t, server.Client(), http.MethodPut, uri+"/bad-KEY", map[string]any{"sandboxId": alternate}, http.StatusConflict)
	suiteAfter := requestJSON(t, server.Client(), http.MethodPost, api+"/sandboxes/"+source+"/tools", nil, http.StatusOK)
	if suiteAfter["providerId"] != pid {
		t.Fatal("workspace target created duplicate Provider")
	}
	for i, item := range suite["tools"].([]any) {
		if item.(map[string]any)["toolId"] != suiteAfter["tools"].([]any)[i].(map[string]any)["toolId"] {
			t.Fatal("workspace target changed Tool IDs")
		}
	}
	requestJSON(t, server.Client(), http.MethodDelete, targetPath, nil, http.StatusNoContent)
	after := requestJSON(t, server.Client(), http.MethodGet, uri, nil, http.StatusOK)
	if len(after["workspaces"].([]any)) != 0 {
		t.Fatal("target not unbound", after)
	}
}
