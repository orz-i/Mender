package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestAgentWorkspaceAdmissionFencesExactApprovedDirectoryAgainstRebind(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "fence.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	root := t.TempDir()
	one := t.TempDir()
	two := t.TempDir()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "Authorized agent", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main",
		WorkspaceRoot: root, Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	profileID := "prof_0123456789abcdef0123456789abcdef"
	if err := store.GrantProfile(ctx, id, profileID); err != nil {
		t.Fatal(err)
	}
	if err := store.BindWorkspace(ctx, id, "project-b", one); err != nil {
		t.Fatal(err)
	}
	if err := store.GrantWorkspace(ctx, id, "project-b", profileID); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	snapshot := agentdomain.Task{ID: taskID, AgentID: id, Status: agentdomain.TaskQueued, Origin: "mcp",
		PrincipalKind: "service", PrincipalRef: "profile:" + profileID,
		ScopeKind: "profile", ScopeRef: profileID,
		Generation: 1, WorkspaceKey: "project-b", WorkspaceRoot: one, CreatedAt: now, UpdatedAt: now}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := store.UnbindWorkspace(ctx, id, "project-b"); err != nil {
		t.Fatal(err)
	}
	if err := store.BindWorkspace(ctx, id, "project-b", two); err != nil {
		t.Fatal(err)
	}
	if err := store.GrantWorkspace(ctx, id, "project-b", profileID); err != nil {
		t.Fatal(err)
	}
	// All identity and grant predicates are true; only the admitted absolute
	// root differs. No task reservation may be created for the old directory.
	if err := store.CreateAdmittedTask(ctx, snapshot, 2, profileID); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("stale target root accepted or rejected incorrectly: %v", err)
	}
	if _, err := store.GetTask(ctx, taskID); !errors.Is(err, agentdomain.ErrNotFound) {
		t.Fatalf("stale-root task was persisted: %v", err)
	}
	snapshot.WorkspaceRoot = two
	if err := store.CreateAdmittedTask(ctx, snapshot, 2, profileID); err != nil {
		t.Fatalf("approved replacement root was rejected: %v", err)
	}
	persisted, err := store.GetTask(ctx, taskID)
	if err != nil || persisted.WorkspaceRoot != two || persisted.WorkspaceKey != "project-b" {
		t.Fatalf("Task snapshot mismatched: %+v %v", persisted, err)
	}
}
