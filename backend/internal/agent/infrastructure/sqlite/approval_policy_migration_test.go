package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestAgentV11ToV12ApprovalModeMigrationPreservesHistoryAndDefaultsOff(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v11-to-v12.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Truncate(time.Millisecond)
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Existing host unchanged", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main",
		WorkspaceRoot: root, Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	task := agentdomain.Task{
		ID: taskID, AgentID: id, Status: agentdomain.TaskQueued,
		WorkspaceRoot: root, Origin: "cli", PrincipalKind: "local_user", PrincipalRef: "os:501",
		ScopeKind: "catalog", Generation: 1,
		CreatedAt: created, UpdatedAt: created,
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, taskID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, taskID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "existing result", ""); err != nil {
		t.Fatal(err)
	}
	aid, err := agentdomain.NewApprovalID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO agent_task_approvals(
 approval_id,task_id,agent_id,generation,session_id,tool_call_id,
 action_kind,target_path,can_allow,allow_option_id,reject_option_id,status,
 created_at,expires_unix_ms,resolved_at,decision_source) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		aid, taskID, id, 1, "old-session", "old-call", "read", "existing.txt", 1,
		"once", "no", "allowed_once", created.Format(time.RFC3339Nano), created.Add(time.Minute).UnixMilli(), created.Format(time.RFC3339Nano), "owner"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DELETE FROM agent_schema_migrations WHERE version=13",
		"DELETE FROM agent_schema_migrations WHERE version=12",
		"DROP TRIGGER agent_workspace_approval_policy_unbind",
		"DROP TABLE agent_approval_policies",
		"DROP TABLE agent_approval_policy_events",
		"ALTER TABLE agent_task_approvals DROP COLUMN decision_source",
	} {
		if _, err := raw.ExecContext(ctx, statement); err != nil {
			t.Fatalf("construct version 11: %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrated.Close() }()
	var version int
	if err := migrated.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 13 {
		t.Fatalf("agent migration did not reach v13: %d %v", version, err)
	}
	found, err := migrated.GetAgent(ctx, id)
	if err != nil || found.Spec().Name != "Existing host unchanged" || found.Spec().Isolation != "host" {
		t.Fatalf("existing owner Host Agent was rewritten %+v %v", found, err)
	}
	got, err := migrated.GetTask(ctx, taskID)
	if err != nil || got.Status != agentdomain.TaskCompleted || got.Result != "existing result" {
		t.Fatalf("existing Task history mutated %+v %v", got, err)
	}
	approval, err := migrated.GetApproval(ctx, aid)
	if err != nil || approval.Status != agentdomain.ApprovalAllowedOnce || approval.DecisionSource != "owner" {
		t.Fatalf("pre-v12 approval evidence changed: %+v %v", approval, err)
	}
	var count int
	if err := migrated.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_approval_policies").Scan(&count); err != nil || count != 0 {
		t.Fatalf("v12 granted implicit unattended mode: %d %v", count, err)
	}
	if err := migrated.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_approval_policy_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("v12 introduced a fictitious permission change event: %d %v", count, err)
	}
	if _, err := migrated.ApprovalPolicy(ctx, id, ""); !errors.Is(err, agentdomain.ErrNotFound) {
		t.Fatalf("default manual not represented by absence: %v", err)
	}
	var quickCheck string
	if err := migrated.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&quickCheck); err != nil || quickCheck != "ok" {
		t.Fatalf("SQLite integrity damaged: %s %v", quickCheck, err)
	}
	if _, err := os.Stat(filepath.Join(root, "existing.txt")); !os.IsNotExist(err) {
		t.Fatal("database migration touched host project files")
	}
}
