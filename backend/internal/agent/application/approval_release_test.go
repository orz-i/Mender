package application

import (
	"context"
	"errors"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestOwnerTrustedModeReleasesAlreadyPendingUnreviewableRequest(t *testing.T) {
	ctx := context.Background()
	s, ownerCtx, runner := newApprovalFixture(t)
	runner.unreviewable = true
	a, err := s.Create(ctx, CreateRequest{Name: "Pending known ACP action", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: t.TempDir()}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(ctx, a.ID(), "waiting for configured trust", ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	<-runner.ready
	pending := waitPendingApproval(t, s, task.ID, ownerCtx)
	if pending.Kind != "execute" || pending.CanAllow {
		t.Fatal("fixture approval incorrectly reviewable")
	}
	if _, err := s.DecideApproval(ctx, pending.ID, agentdomain.ApprovalAllowOnce, ownerCtx); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatal("manual review bypassed unsupported operation")
	}
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalTrusted, ownerCtx); err != nil {
		t.Fatal(err)
	}
	n, err := s.AllowPendingWithCurrentPolicy(ctx, a.ID(), "", ownerCtx)
	if err != nil || n != 1 {
		t.Fatalf("owner mode did not release matching pending ACP request: %d %v", n, err)
	}
	select {
	case got := <-runner.decision:
		if got != agentdomain.ApprovalAllowOnce {
			t.Fatal(got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("existing permission waiter was not released")
	}
	_ = waitTask(t, s, task.ID, ownerCtx, agentdomain.TaskCompleted)
	list, err := s.ListApprovals(ctx, task.ID, ownerCtx)
	if err != nil || len(list) != 1 || list[0].DecisionSource != "policy" || list[0].Status != agentdomain.ApprovalAllowedOnce {
		t.Fatalf("released task did not retain immutable policy evidence: %+v %v", list, err)
	}
	if count, err := s.AllowPendingWithCurrentPolicy(ctx, a.ID(), "", ownerCtx); err != nil || count != 0 {
		t.Fatalf("policy replayed terminal request: %d %v", count, err)
	}
}

func TestExpiredTrustedModeAutomaticallyReturnsToManualBehavior(t *testing.T) {
	p := agentdomain.ApprovalPolicy{Mode: agentdomain.ApprovalTrusted, ExpiresAt: time.Now().Add(-time.Second)}
	if p.Allows("edit", false, time.Now()) || p.Allows("execute", false, time.Now()) {
		t.Fatal("expired owner trust was treated as permanent allow")
	}
}
