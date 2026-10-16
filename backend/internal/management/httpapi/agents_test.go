package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
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
		"isolation": "restricted_host", "defaultModel": "example-model",
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
	if get["defaultModel"] != "example-model" {
		t.Fatal("default model not persisted", get)
	}
	updated := requestJSON(t, server.Client(), http.MethodPatch, api+"/agents/"+id+"/settings", map[string]any{"defaultModel": "other-model"}, http.StatusOK)
	if updated["defaultModel"] != "other-model" {
		t.Fatal("settings not changed", updated)
	}
	toolPath := api + "/agents/" + id + "/tools"
	firstTools := requestJSON(t, server.Client(), http.MethodGet, toolPath, nil, http.StatusOK)
	if firstTools["status"] != "ready" || firstTools["suiteId"] != "agent.delegation" ||
		len(firstTools["tools"].([]any)) != 4 || firstTools["providerName"] != "Code Reviewer" {
		t.Fatalf("new Agent should have its own tools without grants: %v", firstTools)
	}
	secondTools := requestJSON(t, server.Client(), http.MethodPost, toolPath, nil, http.StatusOK)
	if secondTools["providerId"] != firstTools["providerId"] {
		t.Fatalf("explicit ensure re-created facade: %v %v", firstTools, secondTools)
	}
	for index, value := range firstTools["tools"].([]any) {
		if value.(map[string]any)["toolId"] != secondTools["tools"].([]any)[index].(map[string]any)["toolId"] {
			t.Fatal("tool identity changed during explicit ensure")
		}
	}
	grantsBefore := requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id+"/profile-grants", nil, http.StatusOK)
	if len(grantsBefore["profileIds"].([]any)) != 0 {
		t.Fatal("tool setup granted Profile permissions")
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
	wsRoot := t.TempDir()
	sandbox := requestJSON(t, server.Client(), http.MethodPost, api+"/sandboxes/local", map[string]any{
		"name": "Extra project", "root": wsRoot, "isolation": "restricted_host",
	}, http.StatusCreated)
	sandboxID := sandbox["sandboxId"].(string)
	workspacePath := api + "/agents/" + id + "/workspaces/other-project"
	requestJSON(t, server.Client(), http.MethodPut, workspacePath, map[string]any{"sandboxId": sandboxID}, http.StatusNoContent)
	requestJSON(t, server.Client(), http.MethodPut, workspacePath+"/profile-grants/"+profileID, nil, http.StatusNoContent)
	workspaces := requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id+"/workspaces", nil, http.StatusOK)
	if len(workspaces["workspaces"].([]any)) != 1 {
		t.Fatal("workspace not bound", workspaces)
	}
	requestJSON(t, server.Client(), http.MethodDelete, workspacePath+"/profile-grants/"+profileID, nil, http.StatusNoContent)
	requestJSON(t, server.Client(), http.MethodDelete, workspacePath, nil, http.StatusNoContent)
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
	// Denied Agent Policy operations must be independently auditable through
	// the owner-only API, without exporting PrincipalRefs or task prompts.
	if _, err := runtime.Policy.SetDefault.Execute(ctx, policydomain.DecisionDeny); err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginCLI, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Agents.Submit(ctx, agentdomain.ID(id), "DONT_LOG_THIS_AGENT_PROMPT", caller); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("default deny failed: %v", err)
	}
	logs := requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id+"/admissions", nil, http.StatusOK)
	events, ok := logs["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("missing Agent admission history: %v", logs)
	}
	event := events[0].(map[string]any)
	if event["status"] != "denied" || event["action"] != "agent.activate" {
		t.Fatalf("wrong event %v", event)
	}
	for _, field := range []string{"principalRef", "scopeRef", "prompt", "result", "credential"} {
		if _, ok := event[field]; ok {
			t.Fatalf("admission audit leaked %s", field)
		}
	}
	requestJSON(t, server.Client(), http.MethodDelete, api+"/agents/"+id, nil, http.StatusNoContent)
	requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id, nil, http.StatusNotFound)
}
