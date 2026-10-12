package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestSandboxToolSuiteHTTPInertReadAndExplicitOwnerMutation(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "suite.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	root := server.URL + "/api/v1"
	catalogue := requestJSON(t, server.Client(), http.MethodGet, root+"/tool-suites", nil, http.StatusOK)
	suites := catalogue["suites"].([]any)
	if len(suites) != 3 {
		t.Fatalf("suite catalogue %+v", catalogue)
	}
	browser := suites[2].(map[string]any)
	if browser["suiteId"] != "sandbox.browser" || browser["status"] != "unavailable" ||
		len(browser["toolSourceKeys"].([]any)) != 0 {
		t.Fatalf("browser suite misadvertised: %+v", browser)
	}
	sandbox := requestJSON(t, server.Client(), http.MethodPost, root+"/sandboxes/local", map[string]any{
		"name": "Sandbox Tool Suite Fixture", "root": t.TempDir(), "isolation": "host",
	}, http.StatusCreated)
	id := sandbox["sandboxId"].(string)
	uri := root + "/sandboxes/" + id + "/tools"
	before := requestJSON(t, server.Client(), http.MethodGet, uri, nil, http.StatusOK)
	if before["status"] != "missing" || before["available"] != true || len(before["tools"].([]any)) != 0 {
		t.Fatalf("unexpected prebinding Tool Suite: %+v", before)
	}
	providers := requestJSON(t, server.Client(), http.MethodGet, root+"/providers", nil, http.StatusOK)
	if len(providers["providers"].([]any)) != 0 {
		t.Fatal("GET suite caused Provider side effects")
	}
	ready := requestJSON(t, server.Client(), http.MethodPost, uri, nil, http.StatusOK)
	if ready["status"] != "ready" || len(ready["tools"].([]any)) != 4 || ready["providerId"] == "" {
		t.Fatalf("Sandbox suite not prepared: %+v", ready)
	}
	repeated := requestJSON(t, server.Client(), http.MethodPost, uri, nil, http.StatusOK)
	if repeated["providerId"] != ready["providerId"] {
		t.Fatal("idempotent ensure changed Provider identity")
	}
	for i, x := range ready["tools"].([]any) {
		y := repeated["tools"].([]any)[i].(map[string]any)
		if x.(map[string]any)["toolId"] != y["toolId"] {
			t.Fatal("idempotent ensure changed ToolID")
		}
	}
	profiles := requestJSON(t, server.Client(), http.MethodGet, root+"/profiles", nil, http.StatusOK)
	if len(profiles["profiles"].([]any)) != 0 {
		t.Fatal("creating suite granted Profile access")
	}
	requestJSON(t, server.Client(), http.MethodDelete, uri, nil, http.StatusNoContent)
	missing := requestJSON(t, server.Client(), http.MethodGet, uri, nil, http.StatusOK)
	if missing["status"] != "missing" || missing["providerId"] != nil {
		t.Fatalf("suite not removed: %+v", missing)
	}
	requestJSON(t, server.Client(), http.MethodGet, root+"/sandboxes/invalid/tools", nil, http.StatusBadRequest)
}
