package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestSandboxManagementListCreateDetailAndPolicyDenied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "sandbox-management.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)
	empty := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := empty["sandboxes"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("initial sandboxes %+v", empty)
	}
	root := t.TempDir()
	created := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local", map[string]any{"name": "Safe local", "root": root, "isolation": "host"}, http.StatusCreated)
	id, ok := created["sandboxId"].(string)
	if !ok || id == "" {
		t.Fatalf("created %+v", created)
	}
	if created["backend"] != "local_workspace" || created["state"] != "ready" || created["isolation"] != "host" {
		t.Fatalf("unexpected sandbox %+v", created)
	}
	detail := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/"+id, nil, http.StatusOK)
	if detail["sandboxId"] != id {
		t.Fatalf("detail %+v", detail)
	}
	list := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := list["sandboxes"].([]any); !ok || len(arr) != 1 {
		t.Fatalf("listing %+v", list)
	}
	requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/invalid", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/sbx_00000000000000000000000000000000", nil, http.StatusNotFound)
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local", map[string]any{"name": "Bad", "root": "relative", "isolation": "host"}, http.StatusBadRequest)
	if _, err := runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{DisplayName: "Deny new sandbox", Enabled: true, Decision: policydomain.DecisionDeny, Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxProvision}}}); err != nil {
		t.Fatal(err)
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local", map[string]any{"name": "Denied", "root": filepath.Join(t.TempDir(), "absent"), "isolation": "host"}, http.StatusForbidden)
	final := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := final["sandboxes"].([]any); !ok || len(arr) != 1 {
		t.Fatalf("denied provision persisted: %+v", final)
	}
}
