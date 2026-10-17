package application

import (
	"context"
	"errors"
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

func TestApprovalPolicyRevisionFencesStaleAutoAndOwnerDecisions(t *testing.T) {
	for _, test := range []struct {
		name string
		auto bool
	}{
		{name: "automatic request falls back to owner inbox", auto: true},
		{name: "owner allow cannot reuse stale admission", auto: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "fenced-permission.db")
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
			entered := make(chan struct{})
			resume := make(chan struct{})
			var admissions atomic.Int32
			admit := func(_ context.Context, _ gov.Context, _ agentdomain.Agent, action gov.Action) error {
				if action == gov.ActionAgentTaskSubmit && admissions.Add(1) == 2 {
					close(entered)
					<-resume
				}
				return nil
			}
			executor := &approvalExecutorFixture{ready: make(chan struct{}), decision: make(chan agentdomain.ApprovalChoice, 1)}
			ownerCtx := owner(t)
			svc, err := NewService(store, executor, admit, ownerCtx.Principal(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = svc.Close() }()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("safe"), 0o600); err != nil {
				t.Fatal(err)
			}
			a, err := svc.Create(ctx, CreateRequest{Name: "Policy revision gate", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: root}, ownerCtx)
			if err != nil {
				t.Fatal(err)
			}
			if test.auto {
				if _, err := svc.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalWorkspaceRead, ownerCtx); err != nil {
					t.Fatal(err)
				}
			}
			task, err := svc.Submit(ctx, a.ID(), "read with fenced policy", ownerCtx)
			if err != nil {
				t.Fatal(err)
			}
			<-executor.ready
			ownerCAS := make(chan error, 1)
			if !test.auto {
				pending := waitPendingApproval(t, svc, task.ID, ownerCtx)
				go func() {
					// This owner allow blocks at the admission gate.
					_, e := svc.DecideApproval(ctx, pending.ID, agentdomain.ApprovalAllowOnce, ownerCtx)
					ownerCAS <- e
				}()
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("Policy admission fence was not reached")
			}
			before, err := policies.CurrentRevision(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := policies.SetDefaultDecision(ctx, policydomain.DecisionDeny, time.Now()); err != nil {
				t.Fatal(err)
			}
			after, err := policies.CurrentRevision(ctx)
			if err != nil || after <= before {
				t.Fatal("Policy mutation did not advance rev", err)
			}
			close(resume)
			if !test.auto {
				select {
				case ownerErr := <-ownerCAS:
					if !errors.Is(ownerErr, agentdomain.ErrConflict) {
						t.Fatalf("stale Policy owner decision was not fenced: %v", ownerErr)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("stale owner allow did not finish")
				}
			}
			pending := waitPendingApproval(t, svc, task.ID, ownerCtx)
			// Wait for the owner CAS to finish in manual variant; in either case a
			// stale allow must never be committed, while rejection remains possible.
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				current, err := store.GetApproval(ctx, pending.ID)
				if err != nil {
					t.Fatal(err)
				}
				if current.Status == agentdomain.ApprovalPending {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if _, err := svc.DecideApproval(ctx, pending.ID, agentdomain.ApprovalReject, ownerCtx); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-executor.decision:
				if got != agentdomain.ApprovalReject {
					t.Fatal("stale Policy allowed native action", got)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("owner rejection did not reach live ACP waiter")
			}
			_ = waitTask(t, svc, task.ID, ownerCtx, agentdomain.TaskCompleted)
			ledger, err := svc.ListApprovals(ctx, task.ID, ownerCtx)
			if err != nil || len(ledger) != 1 || ledger[0].Status != agentdomain.ApprovalRejected {
				t.Fatalf("stale Policy allowed a durable decision: %+v %v", ledger, err)
			}
			if test.auto && ledger[0].DecisionSource != "owner" {
				t.Fatal("automatic path reused stale approval generation")
			}
		})
	}
}

func TestDetachedACPWaiterCannotPersistApproval(t *testing.T) {
	s, _ := newFixtureService(t, fakeExecutor{started: make(chan struct{}), release: make(chan struct{})})
	id, err := agentdomain.NewApprovalID()
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	alreadyDone := make(chan struct{})
	close(alreadyDone)
	w := &pendingApproval{offers: make(chan pendingApprovalHandoff), done: alreadyDone}
	s.mu.Lock()
	s.active[taskID] = func() {}
	s.pending[id] = w
	s.mu.Unlock()
	var called atomic.Bool
	err = s.resolveWaitingApproval(context.Background(), id, taskID, PermissionDecision{Choice: agentdomain.ApprovalAllowOnce},
		func() error { called.Store(true); return nil })
	if !errors.Is(err, agentdomain.ErrConflict) || called.Load() {
		t.Fatalf("detached waiter received or persisted positive approval: err=%v writes=%v", err, called.Load())
	}
}
