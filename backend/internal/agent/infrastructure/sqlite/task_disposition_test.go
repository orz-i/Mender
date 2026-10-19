package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	_ "modernc.org/sqlite"
)

func TestUnknownTaskDispositionDoesNotRewriteExecutionHistory(t *testing.T) {
	s, task, _ := executionEvidenceFixture(t)
	ctx := context.Background()
	if err := s.Transition(ctx, task.ID, task.Generation, agentdomain.TaskRunning, agentdomain.TaskUnknown, "", "Agent outcome unknown after timeout"); err != nil {
		t.Fatal(err)
	}
	before, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.TaskDisposition(ctx, task.ID); err != nil || got != nil {
		t.Fatalf("unexpected historical annotation %+v %v", got, err)
	}
	if err := s.RecordUnknownDisposition(ctx, task.ID, agentdomain.UnknownHandledElsewhere); err != nil {
		t.Fatal(err)
	}
	first, err := s.TaskDisposition(ctx, task.ID)
	if err != nil || first == nil || first.Handling != agentdomain.UnknownHandledElsewhere || first.RecordedAt.IsZero() {
		t.Fatalf("handling %+v %v", first, err)
	}
	if err := s.RecordUnknownDisposition(ctx, task.ID, agentdomain.UnknownHandledElsewhere); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("duplicate disposition accepted %v", err)
	}
	if err := s.RecordUnknownDisposition(ctx, task.ID, agentdomain.UnknownNeedsReview); err != nil {
		t.Fatal(err)
	}
	got, err := s.TaskDisposition(ctx, task.ID)
	if err != nil || got == nil || got.Handling != agentdomain.UnknownNeedsReview {
		t.Fatalf("review reversal %+v %v", got, err)
	}
	after, err := s.GetTask(ctx, task.ID)
	if err != nil || after.Status != agentdomain.TaskUnknown || after.Generation != before.Generation || after.Result != before.Result || after.Diagnostic != before.Diagnostic || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("Task outcome was rewritten %+v %+v %v", before, after, err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_unknown_dispositions WHERE task_id=?", string(task.ID)).Scan(&count); err != nil || count != 2 {
		t.Fatalf("immutable review events n=%d %v", count, err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE agent_unknown_dispositions SET handling='handled_elsewhere' WHERE task_id=?", string(task.ID)); err == nil {
		t.Fatal("disposition event audit was mutable")
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM agent_unknown_dispositions WHERE task_id=?", string(task.ID)); err == nil {
		t.Fatal("disposition event audit was deletable")
	}
}

func TestUnknownTaskDispositionRejectsNonUnknownAndInvalidState(t *testing.T) {
	s, task, _ := executionEvidenceFixture(t)
	ctx := context.Background()
	if err := s.RecordUnknownDisposition(ctx, task.ID, agentdomain.UnknownHandledElsewhere); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("running Task acknowledged: %v", err)
	}
	if err := s.Transition(ctx, task.ID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "done", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUnknownDisposition(ctx, task.ID, agentdomain.UnknownHandledElsewhere); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("completed Task acknowledged as unknown: %v", err)
	}
	if err := s.RecordUnknownDisposition(ctx, task.ID, "mark_complete"); !errors.Is(err, agentdomain.ErrInvalid) {
		t.Fatalf("invalid status accepted: %v", err)
	}
}

func TestUnknownDispositionMigrationV15ToV16PreservesPriorUnknown(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "task-review-migrate.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Historical", Backend: agentdomain.BackendOpenCode, PluginID: agentdomain.BackendOpenCode.PluginID(),
		ExtensionID: "main", WorkspaceRoot: t.TempDir(), Isolation: "host",
		OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now(),
	})
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
	task := agentdomain.Task{ID: tid, AgentID: id, Status: agentdomain.TaskQueued,
		Origin: "cli", PrincipalKind: "local_user", PrincipalRef: "os:501", ScopeKind: "catalog",
		WorkspaceRoot: a.Spec().WorkspaceRoot, Generation: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, tid, 1, agentdomain.TaskQueued, agentdomain.TaskUnknown, "", "pre-migration unknown"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"DROP TRIGGER agent_unknown_disposition_no_update",
		"DROP TRIGGER agent_unknown_disposition_no_delete",
		"DROP TABLE agent_unknown_dispositions",
		"DELETE FROM agent_schema_migrations WHERE version=16",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	got, err := s.GetTask(ctx, tid)
	if err != nil || got.Status != agentdomain.TaskUnknown || got.Diagnostic != "pre-migration unknown" {
		t.Fatalf("migration mutated Task %+v %v", got, err)
	}
	disposition, err := s.TaskDisposition(ctx, tid)
	if err != nil || disposition != nil {
		t.Fatalf("migration invented work resolution %+v %v", disposition, err)
	}
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 16 {
		t.Fatalf("version=%d %v", version, err)
	}
}
