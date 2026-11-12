package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
		"isolation": "restricted_host", "defaultModel": "example-model", "defaultReasoningEffort": "medium",
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
	updated := requestJSON(t, server.Client(), http.MethodPatch, api+"/agents/"+id+"/settings", map[string]any{"defaultModel": "other-model", "defaultReasoningEffort": "high"}, http.StatusOK)
	if updated["defaultModel"] != "other-model" || updated["defaultReasoningEffort"] != "high" {
		t.Fatal("settings not changed", updated)
	}
	toolPath := api + "/agents/" + id + "/tools"
	firstTools := requestJSON(t, server.Client(), http.MethodGet, toolPath, nil, http.StatusOK)
	if firstTools["status"] != "ready" || firstTools["suiteId"] != "agent.delegation" ||
		len(firstTools["tools"].([]any)) != 5 || firstTools["providerId"] != nil {
		t.Fatalf("new Agent should have its own tools without grants: %v", firstTools)
	}
	secondTools := requestJSON(t, server.Client(), http.MethodPost, toolPath, nil, http.StatusOK)
	if secondTools["providerId"] != nil {
		t.Fatalf("Agent Tools acquired a Provider façade: %v", secondTools)
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
		"slug": "agent-grant-fixture", "displayName": "Agent fixture",
	}, http.StatusCreated)
	profileID := profile["profileId"].(string)
	path := api + "/agents/" + id + "/profile-grants/" + profileID
	requestJSON(t, server.Client(), http.MethodPut, path, nil, http.StatusNoContent)
	got := requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id+"/profile-grants", nil, http.StatusOK)
	if len(got["profileIds"].([]any)) != 1 {
		t.Fatal("grant missing", got)
	}
	wsRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := api + "/agents/" + id + "/workspaces/other-project"
	requestJSON(t, server.Client(), http.MethodPut, workspacePath, map[string]any{"workspaceRoot": filepath.Join(wsRoot, "nonexistent")}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPut, workspacePath, map[string]any{"workspaceRoot": wsRoot}, http.StatusNoContent)
	// API callers get stable 409 reasons, including the user's screenshot:
	// the existing Agent default directory is not another workspace.
	for _, item := range []struct{ path, root, code string }{
		{api + "/agents/" + id + "/workspaces/mender", agent["workspaceRoot"].(string), "agent_workspace_default_directory"},
		{workspacePath, t.TempDir(), "agent_workspace_alias_exists"},
		{api + "/agents/" + id + "/workspaces/another-project", wsRoot + string(filepath.Separator), "agent_workspace_directory_exists"},
	} {
		response := requestJSON(t, server.Client(), http.MethodPut, item.path,
			map[string]any{"workspaceRoot": item.root}, http.StatusConflict)
		if response["code"] != item.code {
			t.Fatalf("wrong Agent workspace conflict: expected %s received %+v", item.code, response)
		}
	}
	requestJSON(t, server.Client(), http.MethodPut, workspacePath+"/profile-grants/"+profileID, nil, http.StatusNoContent)
	workspaces := requestJSON(t, server.Client(), http.MethodGet, api+"/agents/"+id+"/workspaces", nil, http.StatusOK)
	if len(workspaces["workspaces"].([]any)) != 1 || workspaces["workspaces"].([]any)[0].(map[string]any)["workspaceRoot"] != wsRoot {
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
	requestJSON(t, server.Client(), http.MethodGet, api+"/agent-tasks/atask_00000000000000000000000000000000/transcript", nil, http.StatusNotFound)
	requestJSON(t, server.Client(), http.MethodGet, api+"/agent-tasks/atask_00000000000000000000000000000000/events", nil, http.StatusNotFound)
	requestJSON(t, server.Client(), http.MethodGet, api+"/agent-tasks/invalid/transcript", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodGet, api+"/agent-tasks/invalid/events", nil, http.StatusBadRequest)
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

func TestAgentTaskEventsSSE(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "agent-sse.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()

	server := httptest.NewServer(New(runtime))
	defer server.Close()

	api := server.URL + "/api/v1"

	// Create an agent
	created := requestJSON(t, server.Client(), http.MethodPost, api+"/agents", map[string]any{
		"name": "SSE Agent", "backend": "codex", "workspaceRoot": t.TempDir(), "isolation": "restricted_host",
	}, http.StatusCreated)
	agentID, err := agentdomain.ParseID(created["agentId"].(string))
	if err != nil {
		t.Fatal(err)
	}

	caller, err := gov.NewContext(gov.OriginCLI, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}

	task, err := runtime.Agents.Submit(ctx, agentID, "Prompt for SSE streaming", caller)
	if err != nil {
		t.Fatal(err)
	}

	// Request SSE stream
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/agent-tasks/"+string(task.ID)+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("expected Cache-Control no-cache, got %s", cc)
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			eventLines = append(eventLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	if len(eventLines) == 0 {
		t.Fatal("expected at least one event from SSE stream")
	}

	// First event should be user_prompt
	var firstEvt agentdomain.TaskEvent
	if err := json.Unmarshal([]byte(eventLines[0]), &firstEvt); err != nil {
		t.Fatalf("failed to parse first event: %v", err)
	}
	if firstEvt.Type != agentdomain.TaskEventUserPrompt {
		t.Fatalf("expected first event to be user_prompt, got %s", firstEvt.Type)
	}

	// Last event should be terminal
	var lastEvt agentdomain.TaskEvent
	if err := json.Unmarshal([]byte(eventLines[len(eventLines)-1]), &lastEvt); err != nil {
		t.Fatalf("failed to parse last event: %v", err)
	}
	if lastEvt.Type != agentdomain.TaskEventTerminal {
		t.Fatalf("expected last event to be terminal, got %s", lastEvt.Type)
	}
}
