package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func fixtureActiveApprovalTask(t *testing.T, s *Store) (agentdomain.Agent, agentdomain.Task) {
	t.Helper()
	ctx := context.Background()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "ACP Fixture", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: t.TempDir(),
		Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	tid, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := agentdomain.Task{ID: tid, AgentID: id, Status: agentdomain.TaskQueued,
		Origin: "cli", PrincipalKind: "local_user", PrincipalRef: "os:501", ScopeKind: "catalog",
		Generation: 1, WorkspaceRoot: a.Spec().WorkspaceRoot, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, tid, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	task.Status = agentdomain.TaskRunning
	return a, task
}
func newFixtureApproval(t *testing.T, a agentdomain.Agent, task agentdomain.Task, call string) agentdomain.Approval {
	t.Helper()
	id, err := agentdomain.NewApprovalID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	return agentdomain.Approval{ID: id, TaskID: task.ID, AgentID: a.ID(), Generation: task.Generation,
		SessionID: "synthetic-session", ToolCallID: call, Kind: "read", Target: "fixture.txt", CanAllow: true, AllowOptionID: "yes-once", RejectOptionID: "no",
		Status: agentdomain.ApprovalPending, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
}
func TestApprovalStoreV10OneShotCASAndImmutableHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "approval-v10.db")
	setupApprovalPolicyFenceFixture(t, path)
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	a, task := fixtureActiveApprovalTask(t, s)
	one := newFixtureApproval(t, a, task, "call-1")
	if err := s.CreateApproval(ctx, one); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateApproval(ctx, one); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("duplicate permission request stored: %v", err)
	}
	two := newFixtureApproval(t, a, task, "call-2")
	if err := s.CreateApproval(ctx, two); err != nil {
		t.Fatal(err)
	}
	three := newFixtureApproval(t, a, task, "call-3")
	if err := s.CreateApproval(ctx, three); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("unbounded permission queue: %v", err)
	}
	if err := s.ResolveApproval(ctx, one.ID, agentdomain.ApprovalAllowedOnce, 1); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.GetApproval(ctx, one.ID)
	if err != nil || resolved.Status != agentdomain.ApprovalAllowedOnce || resolved.ResolvedAt.IsZero() {
		t.Fatalf("missing allowed-once ledger record: %+v %v", resolved, err)
	}
	if err := s.ResolveApproval(ctx, one.ID, agentdomain.ApprovalRejected, 0); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("terminal approval replay accepted: %v", err)
	}
	if err := s.InvalidateApproval(ctx, one.ID); err != nil {
		t.Fatal(err)
	}
	resolved, err = s.GetApproval(ctx, one.ID)
	if err != nil || resolved.Status != agentdomain.ApprovalAllowedOnce {
		t.Fatalf("terminal approval was overwritten: %+v %v", resolved, err)
	}
	if err := s.RequestCancel(ctx, task.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveApproval(ctx, two.ID, agentdomain.ApprovalAllowedOnce, 1); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("cancelled task permit accepted: %v", err)
	}
	if err := s.InvalidateApproval(ctx, two.ID); err != nil {
		t.Fatal(err)
	}
	records, err := s.ListTaskApprovals(ctx, task.ID)
	if err != nil || len(records) != 2 {
		t.Fatalf("bounded ledger query %+v %v", records, err)
	}
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 12 {
		t.Fatalf("missing forward schema v10: %d %v", version, err)
	}
}
func TestApprovalStoreStartupInvalidatesInterruptedACPDecision(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "interrupted.db")
	setupApprovalPolicyFenceFixture(t, path)
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	a, task := fixtureActiveApprovalTask(t, s)
	approval := newFixtureApproval(t, a, task, "interrupt")
	if err := s.CreateApproval(ctx, approval); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	recovered, err := restarted.GetApproval(ctx, approval.ID)
	if err != nil || recovered.Status != agentdomain.ApprovalExpired {
		t.Fatalf("restarted runtime kept a stale authorization: %+v %v", recovered, err)
	}
	if err := restarted.ResolveApproval(ctx, approval.ID, agentdomain.ApprovalAllowedOnce, 1); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("stale approval could be replayed: %v", err)
	}
	if got, err := restarted.GetTask(ctx, task.ID); err != nil || got.Status != agentdomain.TaskRunning {
		t.Fatalf("Task lifecycle was secretly rewritten by approval recovery: %+v %v", got, err)
	}
}
func TestApprovalStoreRejectsUnreviewableNativeActionAtSQLiteCAS(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "approval-reviewability.db")
	setupApprovalPolicyFenceFixture(t, path)
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	agent, task := fixtureActiveApprovalTask(t, s)
	approval := newFixtureApproval(t, agent, task, "native-shell")
	approval.Kind = "execute"
	approval.Target = ""
	approval.CanAllow = false
	if err := s.CreateApproval(ctx, approval); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveApproval(ctx, approval.ID, agentdomain.ApprovalAllowedOnce, 1); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("SQLite allowed an unreviewable operation: %v", err)
	}
	pending, err := s.GetApproval(ctx, approval.ID)
	if err != nil || pending.Status != agentdomain.ApprovalPending {
		t.Fatalf("denied write mutated request state: %+v %v", pending, err)
	}
	if err := s.ResolveApproval(ctx, approval.ID, agentdomain.ApprovalRejected, 0); err != nil {
		t.Fatalf("rejection of unsafe operation failed: %v", err)
	}
}
