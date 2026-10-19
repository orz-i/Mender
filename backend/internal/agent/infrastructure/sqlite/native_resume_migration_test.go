package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	_ "modernc.org/sqlite"
)

// Synthetic v14 -> v15 migration test exercises the exact forward-only schema
// change on an older database, preserving immutable admissions and unknown
// Task outcomes without touching the owner's live database.
func TestAgentNativeResumeV14MigrationPreservesHistoricalAuditAndUnknownTask(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "native-resume-v14.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	aid, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: aid, Name: "Historical Agent", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: t.TempDir(),
		Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAdmission(ctx, AdmissionEvent{AgentID: aid, Action: "agent.task.submit",
		PrincipalKind: "local_user", PrincipalRef: "os:501", Origin: "cli", ScopeKind: "catalog",
		PolicyKind: "allow", PolicyReason: "reviewed", Status: "admitted"}); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := agentdomain.Task{ID: taskID, AgentID: aid, Status: agentdomain.TaskQueued, Origin: "cli",
		PrincipalKind: "local_user", PrincipalRef: "os:501", ScopeKind: "catalog", Generation: 1,
		WorkspaceRoot: a.Spec().WorkspaceRoot, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, taskID, 1, agentdomain.TaskQueued, agentdomain.TaskUnknown, "", "uncertain original"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeNativeResumeFixtureToV14(t, path)
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	events, err := s.ListAdmissions(ctx, aid, 100)
	if err != nil || len(events) != 1 || events[0].Action != "agent.task.submit" ||
		events[0].PolicyReason != "reviewed" {
		t.Fatalf("original audit corrupted %+v %v", events, err)
	}
	previous, err := s.GetTask(ctx, taskID)
	if err != nil || previous.Status != agentdomain.TaskUnknown || previous.Diagnostic != "uncertain original" {
		t.Fatalf("historical unknown changed %+v %v", previous, err)
	}
	if err := s.RecordAdmission(ctx, AdmissionEvent{AgentID: aid, Action: "agent.task.resume",
		PrincipalKind: "local_user", PrincipalRef: "os:501", Origin: "cli", ScopeKind: "catalog",
		PolicyKind: "deny", PolicyReason: "no_owner_grant", Status: "denied"}); err != nil {
		t.Fatal(err)
	}
	events, err = s.ListAdmissions(ctx, aid, 100)
	if err != nil || len(events) != 2 || events[0].Action != "agent.task.resume" ||
		events[1].Action != "agent.task.submit" {
		t.Fatalf("new/old audit lost history %+v %v", events, err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE agent_admissions SET policy_reason='forged' WHERE agent_id=?", string(aid)); err == nil {
		t.Fatal("audit data became mutable after v15")
	}
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 17 {
		t.Fatalf("incorrect migrated version %d %v", version, err)
	}
	var healthy string
	if err := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&healthy); err != nil || healthy != "ok" {
		t.Fatalf("database invalid %s %v", healthy, err)
	}
}

// Reconstructs the exact prior admission CHECK only in disposable fixtures.
func downgradeNativeResumeFixtureToV14(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"DROP TRIGGER agent_continuation_no_update",
		"DROP TRIGGER agent_continuation_no_delete",
		"DROP TABLE agent_task_continuations",
		"DROP TRIGGER agent_admission_no_update",
		"DROP TRIGGER agent_admission_no_delete",
		`CREATE TABLE agent_admissions_v14 (
    event_id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK(action IN ('agent.activate','agent.task.submit','agent.task.cancel')),
    principal_kind TEXT NOT NULL,principal_ref TEXT NOT NULL,
    origin TEXT NOT NULL,scope_kind TEXT NOT NULL,scope_ref TEXT NOT NULL,
    policy_kind TEXT NOT NULL CHECK(policy_kind IN ('allow','deny','require_approval','failed')),
    policy_reason TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('admitted','denied','failed')),
    occurred_at TEXT NOT NULL)`,
		"INSERT INTO agent_admissions_v14 SELECT * FROM agent_admissions",
		"DROP TABLE agent_admissions",
		"ALTER TABLE agent_admissions_v14 RENAME TO agent_admissions",
		"CREATE INDEX agent_admissions_by_agent ON agent_admissions(agent_id,event_id)",
		`CREATE TRIGGER agent_admission_no_update BEFORE UPDATE ON agent_admissions
    BEGIN SELECT RAISE(ABORT,'Agent admission audit immutable'); END`,
		`CREATE TRIGGER agent_admission_no_delete BEFORE DELETE ON agent_admissions
    BEGIN SELECT RAISE(ABORT,'Agent admission audit immutable'); END`,
		"DELETE FROM agent_schema_migrations WHERE version=17",
		"DELETE FROM agent_schema_migrations WHERE version=15",
	} {
		if _, err := raw.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("synthesize v14 %q: %v", stmt, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentResumeMigrationRejectsUnreviewedAdmissionTrigger(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "unexpected-agent-audit-trigger.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeNativeResumeFixtureToV14(t, path)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TRIGGER external_agent_policy BEFORE INSERT ON agent_admissions
      BEGIN SELECT RAISE(ABORT,'external invariant'); END`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(ctx, path)
	if err == nil {
		_ = opened.Close()
		t.Fatal("v15 discarded an unknown admission trigger")
	}
	if !strings.Contains(err.Error(), "refuses unreviewed admission schema object") {
		t.Fatalf("migration failed for wrong reason: %v", err)
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var version, trigger int
	if err := raw.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 14 {
		t.Fatalf("schema partially upgraded: %d %v", version, err)
	}
	if err := raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
      WHERE type='trigger' AND name='external_agent_policy'`).Scan(&trigger); err != nil || trigger != 1 {
		t.Fatalf("unknown safety trigger was removed: %d %v", trigger, err)
	}
}
