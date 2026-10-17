package application

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	policysqlite "github.com/orz-i/Mender/backend/internal/policy/infrastructure/sqlite"
)

func TestPendingPolicyReleaseRejectsStaleGovernanceGeneration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "policy-pending-race.db")
	policies, err := policysqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = policies.Close() }()
	store, err := agentsqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	entered, resume := make(chan struct{}), make(chan struct{})
	var called atomic.Int32
	admit := func(_ context.Context, _ gov.Context, _ agentdomain.Agent, action gov.Action) error {
		if action == gov.ActionAgentTaskSubmit && called.Add(1) == 2 {
			close(entered)
			<-resume
		}
		return nil
	}
	exec := &approvalExecutorFixture{ready: make(chan struct{}), decision: make(chan agentdomain.ApprovalChoice, 1)}
	ownerCtx := owner(t)
	svc, err := NewService(store, exec, admit, ownerCtx.Principal(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	agent, err := svc.Create(ctx, CreateRequest{Name: "Pending revision fixture", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: root}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := svc.Submit(ctx, agent.ID(), "read pending", ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	<-exec.ready
	pending := waitPendingApproval(t, svc, task.ID, ownerCtx)
	if _, err := svc.SetApprovalPolicy(ctx, agent.ID(), "", agentdomain.ApprovalWorkspaceRead, ownerCtx); err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		n   int
		err error
	}, 1)
	go func() {
		n, e := svc.AllowPendingWithCurrentPolicy(ctx, agent.ID(), "", ownerCtx)
		result <- struct {
			n   int
			err error
		}{n, e}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("pending policy approval did not reach admission")
	}
	if err := policies.SetDefaultDecision(ctx, policydomain.DecisionDeny, time.Now()); err != nil {
		t.Fatal(err)
	}
	close(resume)
	select {
	case outcome := <-result:
		if outcome.n != 0 || outcome.err != nil {
			t.Fatalf("stale pending Policy approved: count=%d err=%v", outcome.n, outcome.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stale pending release never returned")
	}
	current, err := store.GetApproval(ctx, pending.ID)
	if err != nil || current.Status != agentdomain.ApprovalPending {
		t.Fatalf("stale pending Policy wrote a positive receipt: %+v %v", current, err)
	}
	if _, err := svc.DecideApproval(ctx, pending.ID, agentdomain.ApprovalReject, ownerCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case choice := <-exec.decision:
		if choice != agentdomain.ApprovalReject {
			t.Fatal(choice)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request could not return to manual owner")
	}
	_ = waitTask(t, svc, task.ID, ownerCtx, agentdomain.TaskCompleted)
}
