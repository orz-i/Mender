package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestAgentHTTPManagementAndGrantLifecycle(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "agent-management.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	api := server.URL + "/api/v1"
	base := requestJSON(t, server.Client(), http.MethodGet, api+"/agents", nil, http.StatusOK)
	if len(base["agents"].([]any)) != 0 {
		t.Fatal("implicit Agent resources", base)
	}
	requestJSON(t, server.Client(), http.MethodPost, api+"/agents", map[string]any{
		"name": "Illegal", "backend": "shell", "workspaceRoot": t.TempDir()}, http.StatusBadRequest)
	agent := requestJSON(t, server.Client(), http.MethodPost, api+"/agents", map[string]any{
		"name": "Code Reviewer", "backend": "codex", "workspaceRoot": t.TempDir(),
		"isolation": "restricted_host",
	}, http.StatusCreated)
	id := agent["agentId"].(string)
	if agent["backend"] != "codex" || agent["pluginId"] != "io.mender.agent.codex" || agent["backendAvailable"] == nil {
		t.Fatalf("invalid Agent projection %+v", agent)
	}
	for _, field := range []string{"ownerRef", "ownerKind", "credential", "principalRef"} {
		if _, ok := agent[field]; ok {
			t.Fatalf("exposed sensitive Agent metadata: %s", field)
		}
	}
	get := requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id, nil, http.StatusOK)
	if get["agentId"] != id {
		t.Fatal("get Agent mismatch", get)
	}
	profile := requestJSON(t, server.Client(), http.MethodPost, api+"/profiles", map[string]any{
		"slug": "agent-grant-fixture", "displayName": "Agent fixture", "mode": "dynamic",
	}, http.StatusCreated)
	profileID := profile["profileId"].(string)
	path := api + "/agents/" + id + "/profile-grants/" + profileID
	requestJSON(t, server.Client(), http.MethodPut, path, nil, http.StatusNoContent)
	got := requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id+"/profile-grants", nil, http.StatusOK)
	if len(got["profileIds"].([]any)) != 1 {
		t.Fatal("grant missing", got)
	}
	requestJSON(t, server.Client(), http.MethodDelete, path, nil, http.StatusNoContent)
	got = requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id+"/profile-grants", nil, http.StatusOK)
	if len(got["profileIds"].([]any)) != 0 {
		t.Fatal("grant still present", got)
	}
	tasks := requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id+"/tasks", nil, http.StatusOK)
	if len(tasks["tasks"].([]any)) != 0 {
		t.Fatal("unexpected Task", tasks)
	}
	requestJSON(t, server.Client(), http.MethodGet, api+"/agent-tasks/atask_00000000000000000000000000000000", nil, http.StatusNotFound)
	requestJSON(t, server.Client(), http.MethodPost, api+"/agent-tasks/invalid/cancel", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodDelete, api+"/agents/"+id, nil, http.StatusNoContent)
	requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id, nil, http.StatusNotFound)
}
