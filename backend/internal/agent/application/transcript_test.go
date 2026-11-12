package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/agent/infrastructure/localworkspace"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func TestTaskTranscriptInitialPromptAndEvents(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.sqlite")
	store, err := openAgentV3(t, ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	exec := &fakeExecutor{started: make(chan struct{}), release: make(chan struct{})}
	srv, err := NewService(store, exec, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil },
		owner(t).Principal(),
		func(context.Context, string) error { return nil }, localworkspace.Verifier{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Tester", Backend: agentdomain.BackendCodex,
		PluginID: agentdomain.BackendCodex.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
		OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}

	task, err := srv.Submit(ctx, id, "Test transcript prompt", owner(t))
	if err != nil {
		t.Fatal(err)
	}

	// Verify initial user_prompt event exists immediately
	events, err := srv.GetTaskTranscript(ctx, task.ID, owner(t))
	if err != nil {
		t.Fatalf("failed to get task transcript: %v", err)
	}
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event (user_prompt), got %d", len(events))
	}
	if events[0].Type != agentdomain.TaskEventUserPrompt {
		t.Fatalf("expected user_prompt event type, got %s", events[0].Type)
	}
	if events[0].Payload["prompt"] != "Test transcript prompt" {
		t.Fatalf("expected prompt payload, got %+v", events[0].Payload)
	}

	// Release executor
	<-exec.started
	close(exec.release)

	// Wait for task to finish
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		observed, err := srv.Task(ctx, task.ID, owner(t))
		if err == nil && observed.Status.Terminal() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Check transcript has terminal event
	updatedEvents, err := srv.GetTaskTranscript(ctx, task.ID, owner(t))
	if err != nil {
		t.Fatal(err)
	}
	last := updatedEvents[len(updatedEvents)-1]
	if last.Type != agentdomain.TaskEventTerminal {
		t.Fatalf("expected last event to be terminal, got %s", last.Type)
	}
	if last.Payload["status"] != string(agentdomain.TaskCompleted) {
		t.Fatalf("expected status completed, got %+v", last.Payload)
	}
}
