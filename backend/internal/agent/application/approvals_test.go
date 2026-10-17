package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type approvalExecutorFixture struct {
	decision chan agentdomain.ApprovalChoice
	ready    chan struct{}
}

func (x *approvalExecutorFixture) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, config RunConfig) (string, error) {
	if config.PermissionRequester == nil {
		return "", errors.New("missing host approval boundary")
	}
	close(x.ready)
	result, err := config.PermissionRequester(ctx, PermissionRequest{
		SessionID: "fixture-session", ToolCallID: "fixture-call", Kind: "edit",
		AllowOptionID: "permit-once", RejectOptionID: "deny-once",
	})
	if err != nil {
		return "", err
	}
	x.decision <- result.Choice
	return "fixture decision observed", nil
}
func waitPendingApproval(t *testing.T, s *Service, taskID agentdomain.TaskID, ownerCtx gov.Context) agentdomain.Approval {
	t.Helper()
	deadline := time.After(4 * time.Second)
	for {
		items, err := s.ListApprovals(context.Background(), taskID, ownerCtx)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 1 && items[0].Status == agentdomain.ApprovalPending {
			return items[0]
		}
		select {
		case <-deadline:
			t.Fatalf("approval not persisted: %+v", items)
		case <-time.After(15 * time.Millisecond):
		}
	}
}
func TestExplicitOwnerApprovalAllowsOnceWithoutProfileImpersonation(t *testing.T) {
	ctx := context.Background()
	db, err := agentsqlite.Open(ctx, filepath.Join(t.TempDir(), "owner-approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	exec := &approvalExecutorFixture{ready: make(chan struct{}), decision: make(chan agentdomain.ApprovalChoice, 1)}
	ownerCtx := owner(t)
	service, err := NewService(db, exec, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil },
		ownerCtx.Principal(), func(context.Context, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	a, err := service.Create(ctx, CreateRequest{Name: "ACP Owner Approval", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Grant(ctx, a.ID(), goodProfileID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	caller := profile(t, goodProfileID)
	task, err := service.Submit(ctx, a.ID(), "safe synthetic permission", caller)
	if err != nil {
		t.Fatal(err)
	}
	<-exec.ready
	approval := waitPendingApproval(t, service, task.ID, ownerCtx)
	if approval.Kind != "edit" || approval.Status != agentdomain.ApprovalPending || approval.SessionID != "fixture-session" {
		t.Fatalf("wrong approval: %+v", approval)
	}
	if _, err := service.ListApprovals(ctx, task.ID, caller); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile can browse owner approvals: %v", err)
	}
	if _, err := service.DecideApproval(ctx, approval.ID, agentdomain.ApprovalAllowOnce, caller); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile self-approved its Agent tool: %v", err)
	}
	after, err := service.DecideApproval(ctx, approval.ID, agentdomain.ApprovalAllowOnce, ownerCtx)
	if err != nil || after.Status != agentdomain.ApprovalAllowedOnce {
		t.Fatalf("owner decision %+v err=%v", after, err)
	}
	select {
	case choice := <-exec.decision:
		if choice != agentdomain.ApprovalAllowOnce {
			t.Fatal(choice)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("owner decision never reached Agent")
	}
	_ = waitTask(t, service, task.ID, ownerCtx, agentdomain.TaskCompleted)
	if _, err := service.DecideApproval(ctx, approval.ID, agentdomain.ApprovalReject, ownerCtx); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("approval replay allowed: %v", err)
	}
	items, err := service.ListApprovals(ctx, task.ID, ownerCtx)
	if err != nil || len(items) != 1 || items[0].Status != agentdomain.ApprovalAllowedOnce {
		t.Fatalf("persisted resolution is incorrect %+v %v", items, err)
	}
}
func TestPendingAgentApprovalCannotOutliveProfileRevocationOrTaskCancel(t *testing.T) {
	ctx := context.Background()
	s, ownerCtx, exec := newApprovalFixture(t)
	a, err := s.Create(ctx, CreateRequest{Name: "revocation", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, a.ID(), goodProfileID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(ctx, a.ID(), "approval fixture", profile(t, goodProfileID))
	if err != nil {
		t.Fatal(err)
	}
	<-exec.ready
	approval := waitPendingApproval(t, s, task.ID, ownerCtx)
	if err := s.Revoke(ctx, a.ID(), goodProfileID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DecideApproval(ctx, approval.ID, agentdomain.ApprovalAllowOnce, ownerCtx); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("revoked Profile allowed: %v", err)
	}
	if err := s.Cancel(ctx, task.ID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	_ = waitTask(t, s, task.ID, ownerCtx, agentdomain.TaskCancelled)
	if _, err := s.DecideApproval(ctx, approval.ID, agentdomain.ApprovalAllowOnce, ownerCtx); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("cancelled task approval allowed: %v", err)
	}
	select {
	case x := <-exec.decision:
		t.Fatalf("approval reached executor after cancel: %v", x)
	default:
	}
}
func newApprovalFixture(t *testing.T) (*Service, gov.Context, *approvalExecutorFixture) {
	t.Helper()
	ctx := context.Background()
	db, err := agentsqlite.Open(ctx, filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	executor := &approvalExecutorFixture{ready: make(chan struct{}), decision: make(chan agentdomain.ApprovalChoice, 1)}
	o := owner(t)
	service, err := NewService(db, executor, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil },
		o.Principal(), func(context.Context, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service, o, executor
}
