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

func TestAgentProviderBindingAndGrantHistoryAreDurablyGuarded(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// The standalone Agent store need not know Provider persistence; a later
	// Runtime composition on this same database installs the cross-domain guard.
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE provider_configs(
		provider_id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL, config_json TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Test Agent", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
		OwnerKind: "local_user", OwnerRef: "os:owner", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantProfile(ctx, id, "prof_test"); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeProfile(ctx, id, "prof_test"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT action FROM agent_grant_events WHERE agent_id=? ORDER BY event_id`, string(id))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatal(err)
		}
		events = append(events, action)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0] != "grant" || events[1] != "revoke" {
		t.Fatalf("grant history=%v", events)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM agent_grant_events WHERE agent_id=?`, string(id)); err == nil {
		t.Fatal("immutable grant history deleted")
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO provider_configs(provider_id,plugin_id,config_json) VALUES(?,?,?)`,
		"prov_test", "io.mender.provider.agent-delegation", `{"agentId":"`+string(id)+`"}`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAgent(ctx, id); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("in-use Agent was deleted: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM provider_configs WHERE provider_id=?`, "prov_test"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAgent(ctx, id); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_grant_events WHERE agent_id=?`, string(id)).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 2 {
		t.Fatalf("grant history lost after Agent delete: %d", retained)
	}
}

func TestAgentTaskBudgetIsAtomicAcrossAgentsAndPrincipals(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	createAgent := func() agentdomain.ID {
		t.Helper()
		id, e := agentdomain.NewID()
		if e != nil {
			t.Fatal(e)
		}
		a, e := agentdomain.NewAgent(agentdomain.Spec{
			ID: id, Name: "Budgeted Agent", Backend: agentdomain.BackendCodex,
			PluginID: agentdomain.BackendCodex.PluginID(), ExtensionID: "main",
			WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
			OwnerKind: "local_user", OwnerRef: "os:owner", CreatedAt: time.Now(),
		})
		if e != nil {
			t.Fatal(e)
		}
		if e = s.CreateAgent(ctx, a); e != nil {
			t.Fatal(e)
		}
		return id
	}
	createTask := func(agentID agentdomain.ID, principal string) agentdomain.Task {
		t.Helper()
		id, e := agentdomain.NewTaskID()
		if e != nil {
			t.Fatal(e)
		}
		now := time.Now()
		return agentdomain.Task{ID: id, AgentID: agentID, Status: agentdomain.TaskQueued, Generation: 1,
			Origin: "cli", PrincipalKind: "local_user", PrincipalRef: principal,
			ScopeKind: "catalog", CreatedAt: now, UpdatedAt: now}
	}
	one, two, three := createAgent(), createAgent(), createAgent()
	first := createTask(one, "os:owner")
	second := createTask(two, "os:owner")
	third := createTask(three, "os:owner")
	if err := s.CreateAdmittedTask(ctx, first, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAdmittedTask(ctx, second, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAdmittedTask(ctx, third, 2, ""); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("principal budget bypassed across distinct Agents: %v", err)
	}
	unrelated := createTask(three, "os:other")
	if err := s.CreateAdmittedTask(ctx, unrelated, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, first.ID, 1, agentdomain.TaskQueued, agentdomain.TaskCompleted, "", ""); err == nil {
		t.Fatal("queued task completed without execution")
	}
	if err := s.Transition(ctx, first.ID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, first.ID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAdmittedTask(ctx, third, 2, ""); err != nil {
		t.Fatalf("budget not returned by terminal transition: %v", err)
	}
}

func TestAgentExpiredRecoveryFencesOldGenerationWithoutReplay(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "expired.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Recovery", Backend: agentdomain.BackendCodex, PluginID: agentdomain.BackendCodex.PluginID(),
		ExtensionID: "main", WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
		OwnerKind: "local_user", OwnerRef: "os:owner", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	task := func(when time.Time) agentdomain.Task {
		t.Helper()
		taskID, e := agentdomain.NewTaskID()
		if e != nil {
			t.Fatal(e)
		}
		return agentdomain.Task{ID: taskID, AgentID: id, Status: agentdomain.TaskQueued,
			Origin: "cli", PrincipalKind: "local_user", PrincipalRef: "os:owner",
			ScopeKind: "catalog", Generation: 1, CreatedAt: when, UpdatedAt: when}
	}
	stale := task(time.Now().Add(-15 * time.Minute))
	recent := task(time.Now().Add(-time.Minute))
	if err := s.CreateTask(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTask(ctx, recent); err != nil {
		t.Fatal(err)
	}
	count, err := s.ReconcileExpired(ctx, time.Now().Add(-12*time.Minute))
	if err != nil || count != 1 {
		t.Fatalf("reconciled %d tasks, err=%v", count, err)
	}
	got, err := s.GetTask(ctx, stale.ID)
	if err != nil || got.Status != agentdomain.TaskUnknown || got.Generation != 2 || got.Result != "" {
		t.Fatalf("reconciled stale task=%+v,err=%v", got, err)
	}
	if err := s.Transition(ctx, stale.ID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("unfenced worker resumed stale task: %v", err)
	}
	stillActive, err := s.GetTask(ctx, recent.ID)
	if err != nil || stillActive.Status != agentdomain.TaskQueued || stillActive.Generation != 1 {
		t.Fatalf("active task was prematurely reconciled: %+v,err=%v", stillActive, err)
	}
}
