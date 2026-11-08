package toolhardcut_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	agent "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	_ "modernc.org/sqlite"
)

// A real Stage E candidate is created first. Only this disposable candidate
// is then populated with restart evidence to assert that typed Store open has
// no hidden ACP, Task, approval, Sandbox lease, or SSH recovery side effects.
// This is not an approval to swap or execute a production candidate.
func TestTypedContextStoresPreserveTaskApprovalAndLeaseEvidence(t *testing.T) {
	ctx := context.Background()
	artifact := stageVerifiedCandidate(t)
	agentStore, err := agentsqlite.OpenTyped(ctx, artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	sandboxStore, err := sandboxsqlite.OpenTyped(ctx, artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	id, err := agent.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agent.NewAgent(agent.Spec{
		ID: id, Name: "Migrated Agent", Backend: agent.BackendGrok,
		PluginID: agent.BackendGrok.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "restricted_host",
		OwnerKind: "local_user", OwnerRef: "os:owner", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := agentStore.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	taskID, err := agent.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	task := agent.Task{
		ID: taskID, AgentID: id, Status: agent.TaskQueued,
		Origin: "cli", PrincipalKind: "local_user", PrincipalRef: "os:owner",
		ScopeKind: "catalog", Generation: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := agentStore.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	// Simulated persisted uncertainty: OpenTyped must NEVER replay or
	// normalize it. The actual Stage E assessor would block migration of
	// such a task until recovery evidence is reviewed.
	db, err := sql.Open("sqlite", artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE agent_tasks SET status='unknown' WHERE task_id=?", taskID); err != nil {
		t.Fatal(err)
	}
	// Simulate an existing running ACP attempt and its pending review. A
	// v3 Store constructor must NOT expire this state before the final F4
	// Runtime has reviewed task-generation authority and recovery.
	pendingTaskID, err := agent.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	pendingTask := task
	pendingTask.ID = pendingTaskID
	if err := agentStore.CreateTask(ctx, pendingTask); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE agent_tasks SET status='running' WHERE task_id=?", pendingTaskID); err != nil {
		t.Fatal(err)
	}
	approvalID, err := agent.NewApprovalID()
	if err != nil {
		t.Fatal(err)
	}
	if err := agentStore.CreateApproval(ctx, agent.Approval{
		ID: approvalID, TaskID: pendingTaskID, AgentID: id, Generation: 1,
		SessionID: "session-fixture", ToolCallID: "tool-fixture", Kind: "read",
		Target: "README.md", CanAllow: true, AllowOptionID: "allow-once",
		RejectOptionID: "deny", Status: agent.ApprovalPending,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed pending Agent approval: %v", err)
	}
	sandboxID := sandbox.ID("sbx_99999999999999999999999999999999")
	sb, err := sandbox.NewSandbox(sandbox.Spec{
		ID: sandboxID, Name: "Migrated Sandbox", Backend: sandbox.BackendLocalWorkspace,
		Binding:   sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace),
		Isolation: sandbox.IsolationHost, LocalRoot: t.TempDir(),
		OwnerKind: "local_user", OwnerRef: "os:owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sandboxStore.Create(ctx, sb); err != nil {
		t.Fatal(err)
	}
	run, err := sandbox.NewRun(sandboxID, 1, sandbox.StateDefined, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := sandboxStore.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	run, err = sandboxStore.TransitionRun(ctx, run, sandbox.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	run, err = sandboxStore.TransitionRun(ctx, run, sandbox.StateReady, "fixture-backend")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := governance.NewPrincipal(governance.PrincipalLocalUser, "os:owner")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := governance.NewContext(governance.OriginCLI, principal, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := sandboxStore.ReserveExecution(ctx, sandboxID, run.Generation, owner, sandbox.CapabilityRead)
	if err != nil || lease == "" {
		t.Fatalf("seed exactly one persisted lease: %v", err)
	}
	if err := agentStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sandboxStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// #nosec G304 -- only a disposable Stage E candidate created by test.
	before, err := os.ReadFile(artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := sha256.Sum256(before)

	agentStore, err = agentsqlite.OpenTyped(ctx, artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	sandboxStore, err = sandboxsqlite.OpenTyped(ctx, artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	storedAgent, err := agentStore.GetAgent(ctx, id)
	if err != nil || storedAgent.ID() != id {
		t.Fatalf("Agent identity changed: %+v %v", storedAgent, err)
	}
	observed, err := agentStore.GetTask(ctx, taskID)
	if err != nil || observed.Status != agent.TaskUnknown || observed.Generation != 1 {
		t.Fatalf("unknown Task was replayed or rewritten: %+v %v", observed, err)
	}
	pending, err := agentStore.GetApproval(ctx, approvalID)
	if err != nil || pending.Status != agent.ApprovalPending || !pending.ResolvedAt.IsZero() {
		t.Fatalf("Agent approval was auto-decided or expired during typed open: %+v %v", pending, err)
	}
	observedPendingTask, err := agentStore.GetTask(ctx, pendingTaskID)
	if err != nil || observedPendingTask.Status != agent.TaskRunning {
		t.Fatalf("running Task was changed by typed open: %+v %v", observedPendingTask, err)
	}
	storedSandbox, err := sandboxStore.Get(ctx, sandboxID)
	if err != nil || storedSandbox.ID() != sandboxID {
		t.Fatalf("Sandbox identity changed: %+v %v", storedSandbox, err)
	}
	observedRun, err := sandboxStore.GetRun(ctx, sandboxID)
	if err != nil || observedRun.Generation != run.Generation || observedRun.State != sandbox.StateReady {
		t.Fatalf("Sandbox run changed across Store open: %+v %v", observedRun, err)
	}
	leases, err := sandboxStore.ActiveLeaseCount(ctx, sandboxID)
	if err != nil || leases != 1 {
		t.Fatalf("existing lease was dropped/duplicated: %d %v", leases, err)
	}
	if err := agentStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sandboxStore.Close(); err != nil {
		t.Fatal(err)
	}

	// #nosec G304 -- test-private candidate, no production paths.
	after, err := os.ReadFile(artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	afterHash := sha256.Sum256(after)
	if beforeHash != afterHash || !bytes.Equal(before, after) {
		t.Fatal("typed Agent/Sandbox constructor changed offline evidence or attempted recovery")
	}
}

func TestTypedContextStoresRefuseLegacyMissingAndPartialSchemas(t *testing.T) {
	ctx := context.Background()
	legacy := emptyRuntimeFixture(t)
	artifact := stageVerifiedCandidate(t)
	openers := []struct {
		name string
		open func(string) error
	}{
		{"Agent", func(path string) error {
			store, err := agentsqlite.OpenTyped(ctx, path)
			if err != nil {
				return err
			}
			return store.Close()
		}},
		{"Sandbox", func(path string) error {
			store, err := sandboxsqlite.OpenTyped(ctx, path)
			if err != nil {
				return err
			}
			return store.Close()
		}},
	}
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	for _, opener := range openers {
		t.Run(opener.name, func(t *testing.T) {
			if err := opener.open(legacy); err == nil {
				t.Fatal("accepted v2 Provider-backed schema")
			}
			if err := opener.open(missing); err == nil {
				t.Fatal("created absent database")
			}
			if _, err := os.Stat(missing); !os.IsNotExist(err) {
				t.Fatal("created absent database")
			}
			if err := opener.open(artifact.CandidatePath); err != nil {
				t.Fatalf("verified v3 schema refused: %v", err)
			}
		})
	}
	db, err := sql.Open("sqlite", artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(21,'test')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, opener := range openers {
		if err := opener.open(artifact.CandidatePath); err == nil {
			t.Fatalf("%s accepted partial/future cross-context schema", opener.name)
		}
	}
}

func TestTypedContextStoresRejectFacadeResurrection(t *testing.T) {
	ctx := context.Background()
	artifact := stageVerifiedCandidate(t)
	db, err := sql.Open("sqlite", artifact.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE sandbox_job_suite_bindings(sandbox_id TEXT,provider_id TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := agentsqlite.OpenTyped(ctx, artifact.CandidatePath); err == nil {
		_ = store.Close()
		t.Fatal("Agent typed Store accepted legacy Jobs Facade")
	}
	if store, err := sandboxsqlite.OpenTyped(ctx, artifact.CandidatePath); err == nil {
		_ = store.Close()
		t.Fatal("Sandbox typed Store accepted legacy Jobs Facade")
	}
	if db, err := hardcut.OpenTypedContextStore(ctx, artifact.CandidatePath); err == nil {
		_ = db.Close()
		t.Fatal("shared typed schema gate accepted legacy Jobs Facade")
	}
}
