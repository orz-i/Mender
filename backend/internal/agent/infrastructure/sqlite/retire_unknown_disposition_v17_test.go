package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	_ "modernc.org/sqlite"
)

// All fixtures use disposable databases; neither the migration nor these tests
// open the owner's Mender database or run any Agent.
func retiredDispositionFixture(t *testing.T) (string, agentdomain.TaskID) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-v17-retirement.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	agentID, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: agentID, Name: "Original Agent", Backend: agentdomain.BackendOpenCode,
		PluginID: agentdomain.BackendOpenCode.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "host",
		OwnerKind: "local_user", OwnerRef: "os:fixture", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := agentdomain.Task{
		ID: taskID, AgentID: agentID, Status: agentdomain.TaskQueued,
		Origin: "cli", PrincipalKind: "local_user", PrincipalRef: "os:fixture",
		ScopeKind: "catalog", WorkspaceRoot: a.Spec().WorkspaceRoot,
		Generation: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, taskID, 1, agentdomain.TaskQueued, agentdomain.TaskUnknown,
		"", "Agent outcome unknown after timeout"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return path, taskID
}

func prepareSyntheticV16(t *testing.T, path string, taskID agentdomain.TaskID) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	// Reconstruct only the short-lived v16 schema in a private fixture.
	for _, stmt := range []string{
		"DELETE FROM agent_schema_migrations WHERE version=17",
		"INSERT INTO agent_schema_migrations(version,applied_at) VALUES(16,'2026-10-18T00:00:00Z')",
		`CREATE TABLE agent_unknown_dispositions (
			event_id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL REFERENCES agent_tasks(task_id) ON DELETE RESTRICT,
			handling TEXT NOT NULL CHECK(handling IN ('handled_elsewhere','needs_review')),
			occurred_at TEXT NOT NULL
		)`,
		`CREATE INDEX agent_unknown_dispositions_by_task ON agent_unknown_dispositions(task_id,event_id)`,
		`CREATE TRIGGER agent_unknown_disposition_no_update BEFORE UPDATE ON agent_unknown_dispositions
			BEGIN SELECT RAISE(ABORT,'Agent unknown disposition history immutable'); END`,
		`CREATE TRIGGER agent_unknown_disposition_no_delete BEFORE DELETE ON agent_unknown_dispositions
			BEGIN SELECT RAISE(ABORT,'Agent unknown disposition history immutable'); END`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("synthetic v16 schema %q: %v", stmt, err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO agent_unknown_dispositions(task_id,handling,occurred_at)
		VALUES(?,'handled_elsewhere','2026-10-18T00:00:00Z')`, string(taskID)); err != nil {
		t.Fatal(err)
	}
}

func assertRetiredDisposition(t *testing.T, s *Store, taskID agentdomain.TaskID) {
	t.Helper()
	ctx := context.Background()
	var version, v16Rows, schemaObjects int
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 17 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_schema_migrations WHERE version=16").Scan(&v16Rows); err != nil || v16Rows != 0 {
		t.Fatalf("retired migration marker retained n=%d err=%v", v16Rows, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE name LIKE 'agent_unknown_disposition%'`).Scan(&schemaObjects); err != nil || schemaObjects != 0 {
		t.Fatalf("retired schema retained n=%d err=%v", schemaObjects, err)
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil || task.Status != agentdomain.TaskUnknown ||
		task.Diagnostic != "Agent outcome unknown after timeout" || task.Result != "" {
		t.Fatalf("original unknown mutated: %+v err=%v", task, err)
	}
	if _, err := s.ResumePlan(ctx, taskID); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("historic unknown gained resume: %v", err)
	}
	var check string
	if err := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		t.Fatalf("retired schema inconsistent: %q err=%v", check, err)
	}
}

func TestRetireUnknownDispositionV17FromExistingV16(t *testing.T) {
	path, taskID := retiredDispositionFixture(t)
	prepareSyntheticV16(t, path, taskID)
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	assertRetiredDisposition(t, s, taskID)
}

func TestRetireUnknownDispositionV17DirectlyFromV15(t *testing.T) {
	path, taskID := retiredDispositionFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM agent_schema_migrations WHERE version=17"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	assertRetiredDisposition(t, s, taskID)
}

func TestRetireUnknownDispositionV17RefusesUnreviewedV16Objects(t *testing.T) {
	path, taskID := retiredDispositionFixture(t)
	prepareSyntheticV16(t, path, taskID)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER extra_unreviewed_disposition_guard
		BEFORE INSERT ON agent_unknown_dispositions
		BEGIN SELECT RAISE(ABORT,'unreviewed invariant'); END`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(context.Background(), path)
	if err == nil {
		_ = opened.Close()
		t.Fatal("unknown safety trigger was silently destroyed")
	}
	if !strings.Contains(err.Error(), "unreviewed retired schema object") {
		t.Fatalf("unsafe retirement not rejected: %v", err)
	}
	verify, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = verify.Close() }()
	var version, oldEvents, extra int
	if err := verify.QueryRow("SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 16 {
		t.Fatalf("partially advanced v16 schema: %d %v", version, err)
	}
	if err := verify.QueryRow("SELECT COUNT(*) FROM agent_unknown_dispositions").Scan(&oldEvents); err != nil || oldEvents != 1 {
		t.Fatalf("old data deleted by rejected migration: %d %v", oldEvents, err)
	}
	if err := verify.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='extra_unreviewed_disposition_guard'`).Scan(&extra); err != nil || extra != 1 {
		t.Fatalf("unreviewed trigger removed: %d %v", extra, err)
	}
}
