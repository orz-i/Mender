package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestTaskTitleV19MigrationKeepsUnknownTasksUnnamed(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-title-v18.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	agentID, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: agentID, Name: "Title migration fixture", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "host", OwnerKind: "local_user",
		OwnerRef: "os:fixture", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	id, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	original := agentdomain.Task{
		ID: id, AgentID: agentID, Status: agentdomain.TaskQueued, Origin: "cli",
		PrincipalKind: "local_user", PrincipalRef: "os:fixture", ScopeKind: "catalog",
		Generation: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := store.CreateTask(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, id, 1, agentdomain.TaskQueued, agentdomain.TaskUnknown, "", "historical uncertainty"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Only this disposable fixture is downgraded to an exact v18 table/marker.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"ALTER TABLE agent_tasks DROP COLUMN title",
		"DELETE FROM agent_schema_migrations WHERE version=19",
	} {
		if _, err := raw.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("synthetic v18: %q: %v", stmt, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := upgraded.GetTask(ctx, id)
	if err != nil || legacy.Title != "" || legacy.Status != agentdomain.TaskUnknown ||
		legacy.Diagnostic != "historical uncertainty" || legacy.Generation != 1 {
		t.Fatalf("migration forged an old title or lost unknown Task: %+v %v", legacy, err)
	}
	var version int
	if err := upgraded.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 19 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	if _, err := upgraded.db.ExecContext(ctx, "UPDATE agent_tasks SET title='invented' WHERE task_id=?", string(id)); err == nil {
		t.Fatal("migration allowed historical terminal Task title to mutate")
	}

	freshID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	fresh := original
	fresh.ID, fresh.Title = freshID, "检查连接体验"
	if err := upgraded.CreateTask(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	loaded, err := restarted.GetTask(ctx, freshID)
	if err != nil || loaded.Title != fresh.Title || loaded.ID != freshID {
		t.Fatalf("Task title lost after restart: %+v %v", loaded, err)
	}
	listed, err := restarted.ListTasks(ctx, agentID)
	if err != nil || len(listed) != 2 || listed[0].Title != fresh.Title || listed[1].Title != "" {
		t.Fatalf("Task title not stable in list: %+v %v", listed, err)
	}
}
