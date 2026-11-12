package binding

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

type testResumeBindingExecutor struct{}

func (testResumeBindingExecutor) ExecuteConfigured(_ context.Context, _ agentdomain.Agent, prompt string, cfg agentapplication.RunConfig) (string, error) {
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

func waitDesktopTaskStatus(t *testing.T, service *Service, taskID, wantStatus string) AgentTask {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task, err := service.GetAgentTask(taskID)
		if err == nil && task.Status == wantStatus {
			return task
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach status %s within deadline", taskID, wantStatus)
	return AgentTask{}
}

func TestDesktopSubmitAndResumeAgentTask(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "desktop-submit-resume.db"),
		AgentExecutor: testResumeBindingExecutor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()

	service := NewService(ctx, runtime)

	// Create host agent
	created, err := service.CreateAgent(CreateAgentInput{
		Name:          "Host Desktop Agent",
		Backend:       "opencode",
		WorkspaceRoot: t.TempDir(),
		Isolation:     "host",
	})
	if err != nil {
		t.Fatal(err)
	}
	agentID := created.AgentID

	// Validation tests for SubmitAgentTask
	if _, err := service.SubmitAgentTask("invalid", "prompt", SubmitTaskOptions{}); err == nil {
		t.Fatal("expected error for invalid agent ID")
	}
	if _, err := service.SubmitAgentTask(agentID, "", SubmitTaskOptions{}); err == nil {
		t.Fatal("expected error for empty prompt")
	}
	if _, err := service.SubmitAgentTask(agentID, "prompt", SubmitTaskOptions{TimeoutMinutes: 15}); err == nil {
		t.Fatal("expected error for invalid timeout minutes")
	}

	// Valid SubmitAgentTask
	task, err := service.SubmitAgentTask(agentID, "Initial prompt", SubmitTaskOptions{
		Title:           "Desktop Interactive Task",
		Model:           "example-model",
		ReasoningEffort: "high",
		TimeoutMinutes:  20,
	})
	if err != nil {
		t.Fatalf("failed to submit agent task: %v", err)
	}
	if task.TaskID == "" {
		t.Fatal("expected non-empty taskId")
	}
	if task.AgentID != agentID {
		t.Errorf("expected agentId %s, got %s", agentID, task.AgentID)
	}
	if task.Title != "Desktop Interactive Task" {
		t.Errorf("expected title 'Desktop Interactive Task', got %s", task.Title)
	}
	if task.Model != "example-model" {
		t.Errorf("expected model 'example-model', got %s", task.Model)
	}
	if task.ReasoningEffort != "high" {
		t.Errorf("expected reasoningEffort 'high', got %s", task.ReasoningEffort)
	}
	if task.TimeoutSeconds != 1200 {
		t.Errorf("expected timeoutSeconds 1200, got %d", task.TimeoutSeconds)
	}

	// Wait for completion
	waitDesktopTaskStatus(t, service, task.TaskID, "completed")

	// Validation tests for ResumeAgentTask
	if _, err := service.ResumeAgentTask("invalid", "follow-up"); err == nil {
		t.Fatal("expected error for invalid taskId")
	}
	if _, err := service.ResumeAgentTask("atask_00000000000000000000000000000000", "follow-up"); err == nil {
		t.Fatal("expected error for non-existent taskId")
	}
	if _, err := service.ResumeAgentTask(task.TaskID, ""); err == nil {
		t.Fatal("expected error for empty prompt")
	}

	// Valid ResumeAgentTask
	resumed, err := service.ResumeAgentTask(task.TaskID, "Follow up question")
	if err != nil {
		t.Fatalf("failed to resume agent task: %v", err)
	}
	if resumed.TaskID == "" || resumed.TaskID == task.TaskID {
		t.Errorf("expected new taskId, got %s", resumed.TaskID)
	}
	if resumed.ContinuedFromTaskID != task.TaskID {
		t.Errorf("expected continuedFromTaskId %s, got %s", task.TaskID, resumed.ContinuedFromTaskID)
	}
	if resumed.AgentID != agentID {
		t.Errorf("expected agentId %s, got %s", agentID, resumed.AgentID)
	}

	// Wait for resumed task to complete
	waitDesktopTaskStatus(t, service, resumed.TaskID, "completed")

	// Test non-resumable (restricted_host isolation)
	createdRestricted, err := service.CreateAgent(CreateAgentInput{
		Name:          "Restricted Desktop Agent",
		Backend:       "codex",
		WorkspaceRoot: t.TempDir(),
		Isolation:     "restricted_host",
	})
	if err != nil {
		t.Fatal(err)
	}

	restrTask, err := service.SubmitAgentTask(createdRestricted.AgentID, "restricted prompt", SubmitTaskOptions{})
	if err != nil {
		t.Fatalf("failed to submit restricted task: %v", err)
	}

	waitDesktopTaskStatus(t, service, restrTask.TaskID, "completed")

	_, err = service.ResumeAgentTask(restrTask.TaskID, "attempt resume")
	if !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("expected ErrResumeUnavailable, got: %v", err)
	}
}
