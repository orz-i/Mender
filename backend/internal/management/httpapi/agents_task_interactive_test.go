package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

type testResumeExecutor struct{}

func (testResumeExecutor) ExecuteConfigured(_ context.Context, _ agentdomain.Agent, prompt string, cfg agentapplication.RunConfig) (string, error) {
	event := agentdomain.ExecutionEvent{
		Kind:          agentdomain.EventSessionReady,
		NativeSession: "test-session-id",
		CanLoad:       true,
		CanResume:     true,
	}
	if cfg.Resume != nil {
		event.NativeSession = cfg.Resume.NativeSessionID
		event.ExistingSessionRef = cfg.Resume.SessionRef
	}
	for _, e := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 1234},
		event,
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := cfg.Observe(e); err != nil {
			return "", err
		}
	}
	return "done: " + prompt, nil
}

func waitHTTPTaskStatus(t *testing.T, client *http.Client, api, taskID, wantStatus string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task := requestJSON(t, client, http.MethodGet, api+"/agent-tasks/"+taskID, nil, http.StatusOK)
		if task["status"] == wantStatus {
			return task
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach status %s within deadline", taskID, wantStatus)
	return nil
}

func TestAgentTaskSubmitAndResumeHTTP(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "agent-submit-resume.db"),
		AgentExecutor: testResumeExecutor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()

	server := httptest.NewServer(New(runtime))
	defer server.Close()

	api := server.URL + "/api/v1"

	// 1. Create a host-isolated agent (resumable)
	createdHost := requestJSON(t, server.Client(), http.MethodPost, api+"/agents", map[string]any{
		"name":          "Host Agent",
		"backend":       "opencode",
		"workspaceRoot": t.TempDir(),
		"isolation":     "host",
	}, http.StatusCreated)
	hostAgentID := createdHost["agentId"].(string)

	// Validation tests on POST /api/v1/agents/{agentId}/tasks
	requestJSON(t, server.Client(), http.MethodPost, api+"/agents/invalid/tasks", map[string]any{
		"prompt": "some prompt",
	}, http.StatusBadRequest)

	requestJSON(t, server.Client(), http.MethodPost, api+"/agents/agent_00000000000000000000000000000000/tasks", map[string]any{
		"prompt": "some prompt",
	}, http.StatusNotFound)

	requestJSON(t, server.Client(), http.MethodPost, api+"/agents/"+hostAgentID+"/tasks", map[string]any{
		"prompt": "",
	}, http.StatusBadRequest)

	requestJSON(t, server.Client(), http.MethodPost, api+"/agents/"+hostAgentID+"/tasks", map[string]any{
		"prompt":         "some prompt",
		"timeoutMinutes": 15,
	}, http.StatusBadRequest)

	// Valid submission
	taskRes := requestJSON(t, server.Client(), http.MethodPost, api+"/agents/"+hostAgentID+"/tasks", map[string]any{
		"prompt":          "First interactive prompt",
		"title":           "My Interactive Task",
		"model":           "example-model",
		"reasoningEffort": "medium",
		"timeoutMinutes":  10,
	}, http.StatusCreated)

	taskID, ok := taskRes["taskId"].(string)
	if !ok || taskID == "" {
		t.Fatalf("expected valid taskId in response: %+v", taskRes)
	}
	if taskRes["agentId"] != hostAgentID {
		t.Errorf("expected agentId %s, got %v", hostAgentID, taskRes["agentId"])
	}
	if taskRes["title"] != "My Interactive Task" {
		t.Errorf("expected title 'My Interactive Task', got %v", taskRes["title"])
	}
	if taskRes["model"] != "example-model" {
		t.Errorf("expected model 'example-model', got %v", taskRes["model"])
	}
	if taskRes["reasoningEffort"] != "medium" {
		t.Errorf("expected reasoningEffort 'medium', got %v", taskRes["reasoningEffort"])
	}
	if taskRes["timeoutSeconds"] != float64(600) {
		t.Errorf("expected timeoutSeconds 600, got %v", taskRes["timeoutSeconds"])
	}

	// Wait for task to complete
	waitHTTPTaskStatus(t, server.Client(), api, taskID, "completed")

	// Validation tests on POST /api/v1/agent-tasks/{taskId}/resume
	requestJSON(t, server.Client(), http.MethodPost, api+"/agent-tasks/invalid/resume", map[string]any{
		"prompt": "follow up",
	}, http.StatusBadRequest)

	requestJSON(t, server.Client(), http.MethodPost, api+"/agent-tasks/atask_00000000000000000000000000000000/resume", map[string]any{
		"prompt": "follow up",
	}, http.StatusNotFound)

	requestJSON(t, server.Client(), http.MethodPost, api+"/agent-tasks/"+taskID+"/resume", map[string]any{
		"prompt": "",
	}, http.StatusBadRequest)

	// Valid resume
	resumedTask := requestJSON(t, server.Client(), http.MethodPost, api+"/agent-tasks/"+taskID+"/resume", map[string]any{
		"prompt": "Follow up question",
	}, http.StatusCreated)

	resumedTaskID, ok := resumedTask["taskId"].(string)
	if !ok || resumedTaskID == "" {
		t.Fatalf("expected valid taskId in resumed response: %+v", resumedTask)
	}
	if resumedTaskID == taskID {
		t.Errorf("resumed task should have different taskId from parent")
	}
	if resumedTask["continuedFromTaskId"] != taskID {
		t.Errorf("expected continuedFromTaskId %s, got %v", taskID, resumedTask["continuedFromTaskId"])
	}
	if resumedTask["agentId"] != hostAgentID {
		t.Errorf("expected agentId %s, got %v", hostAgentID, resumedTask["agentId"])
	}

	// Wait for resumed task to complete
	waitHTTPTaskStatus(t, server.Client(), api, resumedTaskID, "completed")

	// 2. Test non-resumable (restricted_host isolation)
	createdRestricted := requestJSON(t, server.Client(), http.MethodPost, api+"/agents", map[string]any{
		"name":          "Restricted Agent",
		"backend":       "codex",
		"workspaceRoot": t.TempDir(),
		"isolation":     "restricted_host",
	}, http.StatusCreated)
	restrAgentID := createdRestricted["agentId"].(string)

	restrTask := requestJSON(t, server.Client(), http.MethodPost, api+"/agents/"+restrAgentID+"/tasks", map[string]any{
		"prompt": "restricted task",
	}, http.StatusCreated)
	restrTaskID := restrTask["taskId"].(string)

	waitHTTPTaskStatus(t, server.Client(), api, restrTaskID, "completed")

	// Attempt resume on restricted task -> 409 Conflict
	requestJSON(t, server.Client(), http.MethodPost, api+"/agent-tasks/"+restrTaskID+"/resume", map[string]any{
		"prompt": "attempt resume",
	}, http.StatusConflict)
}
