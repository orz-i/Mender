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

func TestDaytonaManagementCannotCreateBillableResourceWithoutCredential(t *testing.T) {
	t.Setenv(bootstrap.DaytonaExperimentalEnvironment, "enabled")
	t.Setenv("DAYTONA_JWT_TOKEN", "")
	t.Setenv("DAYTONA_ORGANIZATION_ID", "")
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "daytona-api.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)
	backends := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/backends", nil, http.StatusOK)
	if backends["daytonaConfigured"] != true {
		t.Fatalf("adapter should be installed, not credential verified %+v", backends)
	}
	// #nosec G101 -- this is an opaque Mender Credential Reference, never an API secret.
	payload := map[string]any{
		"name": "No secret", "credentialRef": "credential://daytona/api-key",
		"imageDigest": "docker.io/library/python@sha256:" + strings.Repeat("a", 64),
		"target":      "us", "cpu": 1, "memoryGiB": 2, "diskGiB": 3, "ttlMinutes": 30,
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/daytona", payload, http.StatusForbidden)
	payload["ttlMinutes"] = 0
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/daytona", payload, http.StatusBadRequest)
	result := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	items := result["sandboxes"].([]any)
	if len(items) != 0 {
		t.Fatalf("unauthorized Daytona resource persisted: %+v", items)
	}
}

// Without the explicit operator opt-in, even a valid-shaped Credential
// Reference cannot trigger a billable remote request or persistent allocation.
func TestDaytonaManagementDisabledByDefault(t *testing.T) {
	t.Setenv(bootstrap.DaytonaExperimentalEnvironment, "")
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "daytona-disabled-http.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	backends := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/backends", nil, http.StatusOK)
	if backends["daytonaConfigured"] != false {
		t.Fatalf("unverified Daytona enabled by default: %+v", backends)
	}
	status, ok := backends["daytonaStatus"].(string)
	if !ok || !strings.Contains(status, "MENDER_DAYTONA_EXPERIMENTAL=enabled") {
		t.Fatalf("no deployment opt-in guidance: %+v", backends)
	}
	// #nosec G101 -- a test-only Mender Credential Reference, not secret bytes.
	body := map[string]any{
		"name":          "Denied Remote",
		"credentialRef": "credential://daytona/api-key",
		"imageDigest":   "docker.io/library/python@sha256:" + strings.Repeat("a", 64),
		"target":        "us", "cpu": 1, "memoryGiB": 2, "diskGiB": 3, "ttlMinutes": 30,
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/daytona", body, http.StatusServiceUnavailable)
	list := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if values, ok := list["sandboxes"].([]any); !ok || len(values) != 0 {
		t.Fatalf("disabled Daytona created a resource: %+v", list)
	}
}
