package bootstrap

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// A second Runtime can only recover overdue task metadata. No Agent process
// starts merely because it sees tasks from the first Runtime.
func TestAgentRuntimeStartupReconcilesOnlyOverdueTasks(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "agent-recovery.sqlite")
	first, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	owner := agentOwnerContext(t, first)
	agent, err := first.Agents.Create(ctx, agentapplication.CreateRequest{
		Name: "Recovery Fixture", Backend: agentdomain.BackendGrok,
		WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	task := func(when time.Time) agentdomain.Task {
		t.Helper()
		id, e := agentdomain.NewTaskID()
		if e != nil {
			t.Fatal(e)
		}
		return agentdomain.Task{
			ID: id, AgentID: agent.ID(), Status: agentdomain.TaskQueued,
			Origin: "cli", PrincipalKind: string(owner.Principal().Kind()),
			PrincipalRef: owner.Principal().Reference(), ScopeKind: "catalog",
			Generation: 1, CreatedAt: when, UpdatedAt: when,
		}
	}
	expired := task(time.Now().Add(-15 * time.Minute))
	live := task(time.Now().Add(-time.Minute))
	if err := first.agentStore.CreateTask(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if err := first.agentStore.CreateTask(ctx, live); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	after, err := second.agentStore.GetTask(ctx, expired.ID)
	if err != nil || after.Status != agentdomain.TaskUnknown || after.Generation != 2 {
		t.Fatalf("expired Task was not reconciled without replay: %+v / %v", after, err)
	}
	unchanged, err := second.agentStore.GetTask(ctx, live.ID)
	if err != nil || unchanged.Status != agentdomain.TaskQueued || unchanged.Generation != 1 {
		t.Fatalf("recent Task was incorrectly reconciled: %+v / %v", unchanged, err)
	}
}
