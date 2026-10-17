package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestAutomaticReadRefusesRevokedOriginalProfileGrant(t *testing.T) {
	ctx := context.Background()
	runner := &sequentialReadExecutor{
		first: make(chan struct{}), resume: make(chan struct{}),
		decisions: make(chan agentdomain.ApprovalChoice, 2),
	}
	s, ownerCtx := newFixtureService(t, runner)
	root := t.TempDir()
	for _, name := range []string{"one.txt", "two.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := s.Create(ctx, CreateRequest{Name: "Grant-revocation safety", Backend: agentdomain.BackendGrok, WorkspaceRoot: root}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, a.ID(), goodProfileID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalWorkspaceRead, ownerCtx); err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(ctx, a.ID(), "revoke grant after first read", profile(t, goodProfileID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.first:
	case <-time.After(3 * time.Second):
		t.Fatal("first task-scoped auto-allow failed")
	}
	select {
	case got := <-runner.decisions:
		if got != agentdomain.ApprovalAllowOnce {
			t.Fatal("first request should pass while grant is valid")
		}
	default:
		t.Fatal("first request was never allowed")
	}
	if err := s.Revoke(ctx, a.ID(), goodProfileID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	close(runner.resume)
	_ = waitTask(t, s, task.ID, ownerCtx, agentdomain.TaskFailed)
	select {
	case choice := <-runner.decisions:
		t.Fatalf("second request obtained a decision despite a revoked Profile grant: %v", choice)
	default:
	}
	ledger, err := s.ListApprovals(ctx, task.ID, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) != 1 || ledger[0].DecisionSource != "policy" ||
		ledger[0].Status != agentdomain.ApprovalAllowedOnce {
		t.Fatalf("invalid/revoked grant created second auto approval: %+v", ledger)
	}
}
