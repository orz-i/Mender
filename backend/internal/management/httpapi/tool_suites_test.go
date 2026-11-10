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
	if len(suites) != 4 {
		t.Fatalf("suite catalogue %+v", catalogue)
	}
	browser := suites[3].(map[string]any)
	if browser["suiteId"] != "sandbox.browser" || browser["status"] != "unavailable" ||
		len(browser["toolSourceKeys"].([]any)) != 0 {
		t.Fatalf("browser suite misadvertised: %+v", browser)
	}
	sandbox := requestJSON(t, server.Client(), http.MethodPost, root+"/sandboxes/local", map[string]any{
		"name": "Sandbox Tool Suite Fixture", "root": t.TempDir(), "isolation": "host",
	}, http.StatusCreated)
	id := sandbox["sandboxId"].(string)
	uri := root + "/sandboxes/" + id + "/tools"
	ready := requestJSON(t, server.Client(), http.MethodGet, uri, nil, http.StatusOK)
	if ready["status"] != "ready" || len(ready["tools"].([]any)) != 4 || ready["providerId"] != nil {
		t.Fatalf("Native Sandbox projection: %+v", ready)
	}
	providers := requestJSON(t, server.Client(), http.MethodGet, root+"/providers", nil, http.StatusOK)
	if len(providers["providers"].([]any)) != 0 {
		t.Fatal("Native Tools created Provider")
	}
	profiles := requestJSON(t, server.Client(), http.MethodGet, root+"/profiles", nil, http.StatusOK)
	if len(profiles["profiles"].([]any)) != 0 {
		t.Fatal("Native Tools granted Profile")
	}
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		request, _ := http.NewRequest(method, uri, nil)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("removed %s route unexpectedly returned %d", method, response.StatusCode)
		}
	}
	requestJSON(t, server.Client(), http.MethodGet, root+"/sandboxes/invalid/tools", nil, http.StatusBadRequest)
	jobURI := root + "/sandboxes/" + id + "/job-tools"
	jobState := requestJSON(t, server.Client(), http.MethodGet, jobURI, nil, http.StatusOK)
	if jobState["suiteId"] != "sandbox.jobs" || jobState["available"] != false || jobState["providerId"] != nil {
		t.Fatalf("local Sandbox claimed SSH Jobs: %+v", jobState)
	}
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		request, _ := http.NewRequest(method, jobURI, nil)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("removed %s Jobs route unexpectedly returned %d", method, response.StatusCode)
		}
	}
	requestJSON(t, server.Client(), http.MethodGet, root+"/sandboxes/invalid/job-tools", nil, http.StatusBadRequest)
}
