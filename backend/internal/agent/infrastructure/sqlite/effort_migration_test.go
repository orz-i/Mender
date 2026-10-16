package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	_ "modernc.org/sqlite"
)

// Synthetic downgrade only for the fixture: a real v6 database is migrated
// forward without changing Agent/Task identity or any existing grant.
func TestAgentV6ToV7ReasoningEffortMigrationPreservesIdentity(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v6.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "Existing Grok", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: t.TempDir(),
		Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:501", DefaultModel: "grok-4.5", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantProfile(ctx, id, "prof_original"); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	task := agentdomain.Task{ID: taskID, AgentID: id, Status: agentdomain.TaskQueued, Model: "grok-4.5",
		WorkspaceRoot: a.Spec().WorkspaceRoot, Origin: "mcp", PrincipalKind: "service", PrincipalRef: "profile:original",
		ScopeKind: "profile", ScopeRef: "original", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, taskID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, taskID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "done", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"ALTER TABLE agents DROP COLUMN default_reasoning_effort",
		"ALTER TABLE agent_tasks DROP COLUMN reasoning_effort",
		"DELETE FROM agent_schema_migrations WHERE version=7",
		"DELETE FROM agent_schema_migrations WHERE version=8",
	} {
		if _, err := raw.ExecContext(ctx, statement); err != nil {
			t.Fatal("fixture downgrade", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	updated, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = updated.Close() }()
	got, err := updated.GetAgent(ctx, id)
	if err != nil || got.Spec().DefaultModel != "grok-4.5" || got.Spec().DefaultReasoningEffort != "" {
		t.Fatalf("existing Agent modified by migration %+v %v", got.Spec(), err)
	}
	saved, err := updated.GetTask(ctx, taskID)
	if err != nil || saved.Status != agentdomain.TaskCompleted || saved.Model != "grok-4.5" ||
		saved.Result != "done" || saved.ReasoningEffort != "" {
		t.Fatalf("Task history changed %+v %v", saved, err)
	}
	allowed, err := updated.HasProfileGrant(ctx, id, "prof_original")
	if err != nil || !allowed {
		t.Fatalf("Profile AgentGrant removed: %v", err)
	}
	var version int
	if err := updated.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 8 {
		t.Fatalf("migration not applied: v%d %v", version, err)
	}
}
