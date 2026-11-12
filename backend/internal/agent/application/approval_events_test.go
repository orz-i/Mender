package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/agent/infrastructure/localworkspace"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type approvalHookExecutor struct {
	onExecute func(ctx context.Context, config RunConfig)
}

func (e *approvalHookExecutor) ExecuteConfigured(ctx context.Context, agent agentdomain.Agent, prompt string, config RunConfig) (string, error) {
	if e.onExecute != nil {
		e.onExecute(ctx, config)
	}
	return "done", nil
}

func TestApprovalLifecycleEvents(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := openAgentV3(t, ctx, filepath.Join(dir, "mender.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	sampleFile := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(sampleFile, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	execStarted := make(chan struct{})
	allowPermission := make(chan struct{})
	donePermission := make(chan struct{})

	exec := &approvalHookExecutor{
		onExecute: func(ctx context.Context, config RunConfig) {
			close(execStarted)
			<-allowPermission
			// Request permission from the application service
			decision, err := config.PermissionRequester(ctx, PermissionRequest{
				SessionID:      "session_1",
				ToolCallID:     "call_1",
				Kind:           "read",
				Target:         "sample.txt",
				CanAllow:       true,
				AllowOptionID:  "opt_allow",
				RejectOptionID: "opt_reject",
			})
			if err != nil {
				t.Errorf("unexpected permission request error: %v", err)
			}
			if decision.Choice != agentdomain.ApprovalAllowOnce {
				t.Errorf("expected allow_once, got %s", decision.Choice)
			}
			close(donePermission)
		},
	}

	srv, err := NewService(store, exec, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil },
		owner(t).Principal(),
		func(context.Context, string) error { return nil }, localworkspace.Verifier{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	agentID, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: agentID, Name: "ApprovalAgent", Backend: agentdomain.BackendCodex,
		PluginID: agentdomain.BackendCodex.PluginID(), ExtensionID: "main",
		WorkspaceRoot: dir, Isolation: "restricted_host",
		OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}

	task, err := srv.Submit(ctx, agentID, "Task requiring approval", owner(t))
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe to live events
	eventsCh, unsub, err := srv.SubscribeTaskEvents(ctx, task.ID, owner(t))
	if err != nil {
		t.Fatalf("failed to subscribe to task events: %v", err)
	}
	defer unsub()

	// Wait for executor to start
	select {
	case <-execStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for executor to start")
	}

	// Start permission request
	close(allowPermission)

	// Wait for pending approval event on stream
	var pendingApprovalID string
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				t.Fatal("events channel closed unexpectedly")
			}
			if ev.Type == agentdomain.TaskEventApproval && ev.Payload["status"] == "pending" {
				if ev.Payload["target"] != "sample.txt" {
					t.Fatalf("expected target sample.txt, got %v", ev.Payload["target"])
				}
				if ev.Payload["kind"] != "read" {
					t.Fatalf("expected kind read, got %v", ev.Payload["kind"])
				}
				if ev.Payload["approvalId"] == nil || ev.Payload["approvalId"] == "" {
					t.Fatal("expected non-empty approvalId in pending event")
				}
				pendingApprovalID = ev.Payload["approvalId"].(string)
				goto PendingFound
			}
		case <-deadline:
			t.Fatal("timed out waiting for pending approval event")
		}
	}
PendingFound:

	// Decide the approval: allow once
	parsedApprovalID, err := agentdomain.ParseApprovalID(pendingApprovalID)
	if err != nil {
		t.Fatalf("failed to parse approval ID: %v", err)
	}
	decided, err := srv.DecideApproval(ctx, parsedApprovalID, agentdomain.ApprovalAllowOnce, owner(t))
	if err != nil {
		t.Fatalf("failed to decide approval: %v", err)
	}
	if decided.Status != agentdomain.ApprovalAllowedOnce {
		t.Fatalf("expected status allowed_once, got %s", decided.Status)
	}

	// Wait for decided approval event on stream
	deadline = time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				t.Fatal("events channel closed unexpectedly")
			}
			if ev.Type == agentdomain.TaskEventApproval && (ev.Payload["status"] == "allowed" || ev.Payload["decision"] == "allowed") {
				if ev.Payload["approvalId"] != pendingApprovalID {
					t.Fatalf("expected approvalId %s, got %v", pendingApprovalID, ev.Payload["approvalId"])
				}
				goto DecidedFound
			}
		case <-deadline:
			t.Fatal("timed out waiting for decided approval event")
		}
	}
DecidedFound:

	// Wait for permission requester to return and task to finish
	select {
	case <-donePermission:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for permission to complete")
	}

	// Wait for terminal event on stream
	deadline = time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				t.Fatal("events channel closed before terminal event")
			}
			if ev.Type == agentdomain.TaskEventTerminal {
				goto TerminalFound
			}
		case <-deadline:
			t.Fatal("timed out waiting for terminal event on stream")
		}
	}
TerminalFound:

	// Verify transcript in SQLite contains the approval events
	transcript, err := srv.GetTaskTranscript(ctx, task.ID, owner(t))
	if err != nil {
		t.Fatal(err)
	}
	foundPending := false
	foundDecided := false
	for _, ev := range transcript {
		if ev.Type == agentdomain.TaskEventApproval {
			if ev.Payload["status"] == "pending" {
				foundPending = true
			}
			if ev.Payload["status"] == "allowed" {
				foundDecided = true
			}
		}
	}
	if !foundPending {
		t.Fatal("expected pending approval event in transcript")
	}
	if !foundDecided {
		t.Fatal("expected allowed approval event in transcript")
	}
}

func TestApprovalLifecycleEvents_Reject(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := openAgentV3(t, ctx, filepath.Join(dir, "mender-reject.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	sampleFile := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(sampleFile, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	execStarted := make(chan struct{})
	allowPermission := make(chan struct{})
	donePermission := make(chan struct{})

	exec := &approvalHookExecutor{
		onExecute: func(ctx context.Context, config RunConfig) {
			close(execStarted)
			<-allowPermission
			decision, err := config.PermissionRequester(ctx, PermissionRequest{
				SessionID:      "session_reject",
				ToolCallID:     "call_reject",
				Kind:           "read",
				Target:         "sample.txt",
				CanAllow:       true,
				AllowOptionID:  "opt_allow",
				RejectOptionID: "opt_reject",
			})
			if err != nil {
				t.Errorf("unexpected permission request error: %v", err)
			}
			if decision.Choice != agentdomain.ApprovalReject {
				t.Errorf("expected reject, got %s", decision.Choice)
			}
			close(donePermission)
		},
	}

	srv, err := NewService(store, exec, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil },
		owner(t).Principal(),
		func(context.Context, string) error { return nil }, localworkspace.Verifier{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	agentID, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: agentID, Name: "RejectAgent", Backend: agentdomain.BackendCodex,
		PluginID: agentdomain.BackendCodex.PluginID(), ExtensionID: "main",
		WorkspaceRoot: dir, Isolation: "restricted_host",
		OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}

	task, err := srv.Submit(ctx, agentID, "Task requiring approval rejection", owner(t))
	if err != nil {
		t.Fatal(err)
	}

	eventsCh, unsub, err := srv.SubscribeTaskEvents(ctx, task.ID, owner(t))
	if err != nil {
		t.Fatalf("failed to subscribe to task events: %v", err)
	}
	defer unsub()

	select {
	case <-execStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for executor to start")
	}

	close(allowPermission)

	var pendingApprovalID string
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				t.Fatal("events channel closed unexpectedly")
			}
			if ev.Type == agentdomain.TaskEventApproval && ev.Payload["status"] == "pending" {
				pendingApprovalID = ev.Payload["approvalId"].(string)
				goto PendingFound
			}
		case <-deadline:
			t.Fatal("timed out waiting for pending approval event")
		}
	}
PendingFound:

	parsedApprovalID, err := agentdomain.ParseApprovalID(pendingApprovalID)
	if err != nil {
		t.Fatalf("failed to parse approval ID: %v", err)
	}
	decided, err := srv.DecideApproval(ctx, parsedApprovalID, agentdomain.ApprovalReject, owner(t))
	if err != nil {
		t.Fatalf("failed to decide approval: %v", err)
	}
	if decided.Status != agentdomain.ApprovalRejected {
		t.Fatalf("expected status rejected, got %s", decided.Status)
	}

	deadline = time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				t.Fatal("events channel closed unexpectedly")
			}
			if ev.Type == agentdomain.TaskEventApproval && ev.Payload["status"] == "rejected" {
				if ev.Payload["decision"] != "rejected" {
					t.Fatalf("expected decision rejected, got %v", ev.Payload["decision"])
				}
				goto RejectedFound
			}
		case <-deadline:
			t.Fatal("timed out waiting for rejected approval event")
		}
	}
RejectedFound:

	select {
	case <-donePermission:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for permission to complete")
	}
}
