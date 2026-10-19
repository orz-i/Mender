package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestTaskBudgetV18ForwardMigrationPreservesHistoricalUnknown(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-budget-v17.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	agentID, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: agentID, Name: "Original agent", Backend: agentdomain.BackendOpenCode,
		PluginID: agentdomain.BackendOpenCode.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "host",
		OwnerKind: "local_user", OwnerRef: "os:fixture", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	tid, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	task := agentdomain.Task{
		ID: tid, AgentID: agentID, Status: agentdomain.TaskQueued,
		Origin: "cli", PrincipalKind: "local_user", PrincipalRef: "os:fixture",
		ScopeKind: "catalog", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, tid, 1, agentdomain.TaskQueued, agentdomain.TaskUnknown, "", "original uncertainty"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Synthetic fixture only: rebuild an exact v17 shape without the column.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	stripTaskBudgetV18Fixture(t, raw)
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	stored, err := reopened.GetTask(ctx, tid)
	if err != nil || stored.Status != agentdomain.TaskUnknown ||
		stored.Diagnostic != "original uncertainty" || stored.TimeoutSeconds != 600 ||
		stored.Generation != 1 {
		t.Fatalf("v18 mutated immutable historic unknown: %+v err=%v", stored, err)
	}
	var v, n int
	if err := reopened.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&v); err != nil || v != 18 {
		t.Fatalf("migrated version=%d err=%v", v, err)
	}
	if err := reopened.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_tasks WHERE timeout_seconds=600").Scan(&n); err != nil || n != 1 {
		t.Fatalf("historic Task budget changed: %d, %v", n, err)
	}
}

func TestTaskBudgetReconciliationUsesImmutablePerTaskDeadline(t *testing.T) {
	ctx := context.Background()
	s, task, _ := executionEvidenceFixture(t)
	now := time.Now().UTC()
	// Long Tasks have an extended bounded deadline. A restarted Runtime must
	// not reconcile them at the original 12-minute recovery threshold.
	if _, err := s.db.ExecContext(ctx, "UPDATE agent_tasks SET timeout_seconds=1200,updated_at=? WHERE task_id=?",
		now.Add(-13*time.Minute).Format(time.RFC3339Nano), string(task.ID)); err != nil {
		t.Fatal(err)
	}
	count, err := s.ReconcileExpired(ctx, now.Add(-12*time.Minute))
	if err != nil || count != 0 {
		t.Fatalf("20min Task prematurely marked unknown: %d, %v", count, err)
	}
	if got, err := s.GetTask(ctx, task.ID); err != nil || got.Status != agentdomain.TaskRunning || got.TimeoutSeconds != 1200 {
		t.Fatalf("running Task lost budget %+v %v", got, err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE agent_tasks SET updated_at=? WHERE task_id=?",
		now.Add(-24*time.Minute).Format(time.RFC3339Nano), string(task.ID)); err != nil {
		t.Fatal(err)
	}
	count, err = s.ReconcileExpired(ctx, now.Add(-12*time.Minute))
	if err != nil || count != 1 {
		t.Fatalf("truly expired extended Task not reconciled %d %v", count, err)
	}
	got, err := s.GetTask(ctx, task.ID)
	if err != nil || got.Status != agentdomain.TaskUnknown || got.TimeoutSeconds != 1200 ||
		got.Result != "" || got.Generation != task.Generation+1 {
		t.Fatalf("unsafe reconciliation %+v %v", got, err)
	}
	if err := s.Transition(ctx, task.ID, task.Generation, agentdomain.TaskRunning, agentdomain.TaskCompleted, "late", ""); err == nil {
		t.Fatal("old generation overwrote an unknown extended Task")
	}
}
