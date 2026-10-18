package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// Exact v12 agent constraint to exercise real forward-only v13 schema upgrade.
// The source database is synthetic; tests never open the owner's production DB.
func downgradeAntigravityFixtureToV12(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	stripExecutionEvidenceV14Fixture(t, db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range []string{
		`CREATE TABLE agents_v12 (
    agent_id TEXT PRIMARY KEY,name TEXT NOT NULL,backend TEXT NOT NULL,
    plugin_id TEXT NOT NULL,extension_id TEXT NOT NULL,
    workspace_root TEXT NOT NULL,isolation TEXT NOT NULL,
    owner_kind TEXT NOT NULL,owner_ref TEXT NOT NULL,created_at TEXT NOT NULL,
    default_model TEXT NOT NULL DEFAULT '',default_reasoning_effort TEXT NOT NULL DEFAULT '',
    CHECK(backend IN ('codex','grok','opencode')),
    CHECK(isolation IN ('host','restricted_host'))
   )`,
		`INSERT INTO agents_v12 SELECT * FROM agents`,
		`DROP TABLE agents`,
		`ALTER TABLE agents_v12 RENAME TO agents`,
		`DELETE FROM agent_schema_migrations WHERE version=13`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("v12 fixture %q: %v", stmt, err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("synthetic v12 fixture has foreign key violations")
	}
	_ = rows.Close()
}

func TestAntigravityV12ToV13PreservesHistoricalAgentTaskGrantsAndPolicy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v12-to-v13.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	now := time.Now().UTC()
	spec := agentdomain.Spec{ID: id, Name: "Historic Grok", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: root,
		Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: now,
		DefaultModel: "grok-4.7", DefaultReasoningEffort: "xhigh"}
	agent, err := agentdomain.NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	if err = s.GrantProfile(ctx, id, "prof_original"); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	task := agentdomain.Task{ID: taskID, AgentID: id, Status: agentdomain.TaskQueued, Origin: "mcp",
		PrincipalKind: "service", PrincipalRef: "profile:prof_original", ScopeKind: "profile", ScopeRef: "prof_original",
		Generation: 1, Model: spec.DefaultModel, ReasoningEffort: spec.DefaultReasoningEffort, WorkspaceRoot: root, CreatedAt: now, UpdatedAt: now}
	if err = s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, taskID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, taskID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "historic success", ""); err != nil {
		t.Fatal(err)
	}
	trusted := agentdomain.ApprovalPolicy{AgentID: id, WorkspaceRoot: root, Mode: agentdomain.ApprovalTrusted,
		UpdatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	if err = s.SetApprovalPolicy(ctx, trusted); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeAntigravityFixtureToV12(t, path)
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var version int
	if err = s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != latestSchemaVersion {
		t.Fatalf("version=%d err=%v", version, err)
	}
	got, err := s.GetAgent(ctx, id)
	if err != nil || got.Spec().DefaultModel != spec.DefaultModel || got.Spec().DefaultReasoningEffort != spec.DefaultReasoningEffort || got.Spec().PluginID != spec.PluginID {
		t.Fatalf("historical Agent modified: %+v / %v", got.Spec(), err)
	}
	previous, err := s.GetTask(ctx, taskID)
	if err != nil || previous.Status != agentdomain.TaskCompleted || previous.Result != "historic success" {
		t.Fatalf("Task history modified: %+v / %v", previous, err)
	}
	grant, err := s.HasProfileGrant(ctx, id, "prof_original")
	if err != nil || !grant {
		t.Fatalf("lost Profile grant: %v", err)
	}
	mode, err := s.ApprovalPolicy(ctx, id, "")
	if err != nil || mode.Mode != agentdomain.ApprovalTrusted || !mode.ExpiresAt.Equal(time.UnixMilli(trusted.ExpiresAt.UnixMilli())) {
		t.Fatalf("lost trusted authorization: %+v / %v", mode, err)
	}
	var sanity string
	if err = s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&sanity); err != nil || sanity != "ok" {
		t.Fatalf("integrity=%s err=%v", sanity, err)
	}
	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("foreign key integrity lost")
	}
	_ = rows.Close()
	id2, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	spec.ID = id2
	spec.Name = "New Antigravity"
	spec.Backend = agentdomain.BackendAntigravity
	spec.PluginID = spec.Backend.PluginID()
	newer, err := agentdomain.NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateAgent(ctx, newer); err != nil {
		t.Fatalf("new backend missing from CHECK: %v", err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE agents SET backend='unsupported' WHERE agent_id=?", id2); err == nil {
		t.Fatal("invalid backend bypassed v13 CHECK")
	}
}
