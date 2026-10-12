package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestAgentTaskPersistenceFencingAndGrants(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "Reviewer",
		Backend: agentdomain.BackendCodex, PluginID: agentdomain.BackendCodex.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "restricted_host", OwnerKind: "local_user",
		OwnerRef: "os:501", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAgent(ctx, id); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListAgents(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("agents=%+v error=%v", list, err)
	}
	if err := s.GrantProfile(ctx, id, "profile-a"); err != nil {
		t.Fatal(err)
	}
	if yes, err := s.HasProfileGrant(ctx, id, "profile-a"); err != nil || !yes {
		t.Fatal("grant missing", err)
	}
	if err := s.RevokeProfile(ctx, id, "profile-a"); err != nil {
		t.Fatal(err)
	}
	if yes, err := s.HasProfileGrant(ctx, id, "profile-a"); err != nil || yes {
		t.Fatal("grant still active", err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	task := agentdomain.Task{ID: taskID, AgentID: id, Status: agentdomain.TaskQueued,
		Origin: "mcp", PrincipalKind: "service", PrincipalRef: "profile:abc", ScopeKind: "profile", ScopeRef: "abc",
		Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, taskID, 2, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatal("wrong generation succeeded", err)
	}
	if err := s.Transition(ctx, taskID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, taskID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "finished", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, taskID, 1, agentdomain.TaskRunning, agentdomain.TaskFailed, "", "bad"); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatal("terminal task mutated", err)
	}
	got, err := s.GetTask(ctx, taskID)
	if err != nil || got.Status != agentdomain.TaskCompleted || got.Result != "finished" || !got.OwnedBy("service", "profile:abc", "profile", "abc") {
		t.Fatalf("persisted %+v / %v", got, err)
	}
	if err := s.DeleteAgent(ctx, id); err == nil {
		t.Fatal("deleted Agent with active task history")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	if got, err := second.GetTask(ctx, taskID); err != nil || got.Result != "finished" {
		t.Fatal("restart did not preserve terminal task", err)
	}
}
func TestAgentSchemaDowngradeRefused(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "store.db")
	s, err := Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_schema_migrations(version,applied_at) VALUES(99,'future')`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if newer, err := Open(ctx, p); err == nil {
		_ = newer.Close()
		t.Fatal("future schema accepted")
	}
}
