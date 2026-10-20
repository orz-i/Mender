package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type twoConcurrentReadApprovals struct {
	submitSecond chan struct{}
	done         chan agentdomain.ApprovalChoice
}

func (runner *twoConcurrentReadApprovals) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, cfg RunConfig) (string, error) {
	type outcome struct {
		choice agentdomain.ApprovalChoice
		err    error
	}
	responses := make(chan outcome, 2)
	read := func(target string) {
		result, err := cfg.PermissionRequester(ctx, PermissionRequest{
			SessionID: "preview-fairness-session", ToolCallID: target,
			Kind: "read", Target: target, CanAllow: true,
			AllowOptionID: "allow-once", RejectOptionID: "reject-once",
		})
		responses <- outcome{choice: result.Choice, err: err}
	}
	go read("not-found.txt")
	select {
	case <-runner.submitSecond:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	go read("valid.txt")
	for i := 0; i < 2; i++ {
		select {
		case answer := <-responses:
			if answer.err != nil {
				return "", answer.err
			}
			runner.done <- answer.choice
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err := observeFixtureTerminal(cfg); err != nil {
		return "", err
	}
	return "both approvals accounted for", nil
}

func TestMissingReadPreviewDoesNotBlockLaterValidApproval(t *testing.T) {
	ctx := context.Background()
	runner := &twoConcurrentReadApprovals{
		submitSecond: make(chan struct{}),
		done:         make(chan agentdomain.ApprovalChoice, 2),
	}
	s, ownerCtx := newFixtureService(t, runner)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "valid.txt"), []byte("safe fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := s.Create(ctx, CreateRequest{Name: "Fail closed without queue starvation",
		Backend: agentdomain.BackendGrok, WorkspaceRoot: root}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(ctx, a.ID(), "two parallel reads", ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	// Wait until the missing-file request is durable before starting a
	// legitimate read; otherwise concurrent scheduling could reorder them.
	first := waitPendingApproval(t, s, task.ID, ownerCtx)
	if first.Target != "not-found.txt" {
		t.Fatalf("wrong fixture creation order: %+v", first)
	}
	close(runner.submitSecond)
	var approvals []agentdomain.Approval
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		approvals, err = s.ListApprovals(ctx, task.ID, ownerCtx)
		if err != nil {
			t.Fatal(err)
		}
		if len(approvals) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(approvals) != 2 {
		t.Fatalf("second read never became pending: %+v", approvals)
	}
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalWorkspaceRead, ownerCtx); err != nil {
		t.Fatal(err)
	}
	allowed, err := s.AllowPendingWithCurrentPolicy(ctx, a.ID(), "", ownerCtx)
	if err != nil || allowed != 1 {
		t.Fatalf("invalid preview starved the valid read: allowed=%d err=%v", allowed, err)
	}
	approvals, err = s.ListApprovals(ctx, task.ID, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	var badID agentdomain.ApprovalID
	for _, entry := range approvals {
		switch entry.Target {
		case "not-found.txt":
			badID = entry.ID
			if entry.Status != agentdomain.ApprovalPending {
				t.Fatal("missing file was automatically permitted")
			}
		case "valid.txt":
			if entry.Status != agentdomain.ApprovalAllowedOnce || entry.DecisionSource != "policy" {
				t.Fatalf("valid read not automatically allowed: %+v", entry)
			}
		default:
			t.Fatalf("unexpected read target: %q", entry.Target)
		}
	}
	if badID == "" {
		t.Fatal("the unsafe request was dropped instead of staying reviewable")
	}
	if _, err := s.DecideApproval(ctx, badID, agentdomain.ApprovalReject, ownerCtx); err != nil {
		t.Fatal(err)
	}
	_ = waitTask(t, s, task.ID, ownerCtx, agentdomain.TaskCompleted)
	var accepted, rejected int
	for i := 0; i < 2; i++ {
		select {
		case choice := <-runner.done:
			switch choice {
			case agentdomain.ApprovalAllowOnce:
				accepted++
			case agentdomain.ApprovalReject:
				rejected++
			}
		case <-time.After(3 * time.Second):
			t.Fatal("one of the two live decisions was lost")
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("unexpected read decisions: accepted=%d rejected=%d", accepted, rejected)
	}
}
