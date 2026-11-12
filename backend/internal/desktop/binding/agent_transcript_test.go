package binding

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

func TestGetAgentTaskEvents(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "wails-events.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()

	service := NewService(ctx, runtime)

	created, err := service.CreateAgent(CreateAgentInput{
		Name: "Desktop Stream Agent", Backend: "codex", WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
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

	task, err := runtime.Agents.Submit(ctx, aID, "Streaming task prompt", caller)
	if err != nil {
		t.Fatal(err)
	}

	// Fetch events starting from afterSeq 0
	eventsResult, err := service.GetAgentTaskEvents(string(task.ID), 0)
	if err != nil {
		t.Fatalf("failed to get agent task events: %v", err)
	}
	if len(eventsResult.Events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(eventsResult.Events))
	}
	if eventsResult.Events[0].Seq != 1 || eventsResult.Events[0].Type != "user_prompt" {
		t.Fatalf("expected seq 1 user_prompt, got %+v", eventsResult.Events[0])
	}

	// Wait for task to finish and check afterSeq 1
	var subsequentEvents AgentTaskTranscript
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		subsequentEvents, err = service.GetAgentTaskEvents(string(task.ID), 1)
		if err == nil && len(subsequentEvents.Events) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(subsequentEvents.Events) == 0 {
		t.Fatal("expected subsequent events after seq 1")
	}
	for _, ev := range subsequentEvents.Events {
		if ev.Seq <= 1 {
			t.Fatalf("expected event seq > 1, got %d", ev.Seq)
		}
	}

	// Invalid Task ID
	if _, err := service.GetAgentTaskEvents("invalid", 0); err == nil {
		t.Fatal("expected error for invalid task ID")
	}
}
