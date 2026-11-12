package binding

import (
	"context"
	"path/filepath"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func TestGetAgentTaskTranscript(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "wails-transcript.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()

	service := NewService(ctx, runtime)

	created, err := service.CreateAgent(CreateAgentInput{
		Name: "Desktop Agent", Backend: "codex", WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
	})
	if err != nil {
		t.Fatal(err)
	}

	aID, err := agentdomain.ParseID(created.AgentID)
	if err != nil {
		t.Fatal(err)
	}

	caller, err := gov.NewContext(gov.OriginDesktop, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}

	task, err := runtime.Agents.Submit(ctx, aID, "Initial task prompt", caller)
	if err != nil {
		t.Fatal(err)
	}

	transcript, err := service.GetAgentTaskTranscript(string(task.ID))
	if err != nil {
		t.Fatalf("failed to get agent task transcript: %v", err)
	}

	if len(transcript.Events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(transcript.Events))
	}
	first := transcript.Events[0]
	if first.Type != "user_prompt" {
		t.Errorf("expected first event to be user_prompt, got %s", first.Type)
	}
	if first.TaskID != string(task.ID) {
		t.Errorf("expected taskId %s, got %s", task.ID, first.TaskID)
	}
	if first.Payload["prompt"] != "Initial task prompt" {
		t.Errorf("expected prompt in payload, got %+v", first.Payload)
	}

	// Invalid Task ID
	if _, err := service.GetAgentTaskTranscript("invalid"); err == nil {
		t.Fatal("expected error for invalid task ID")
	}
}
