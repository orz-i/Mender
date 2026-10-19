package bootstrap

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	_ "modernc.org/sqlite"
)

// downgradeAgentsToV7Fixture reproduces the actual v7 CHECK constraint in
// an isolated fixture. Production schemas/data are NEVER downgraded.
func downgradeAgentsToV7Fixture(t *testing.T, path string, sandboxID ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, sqlText := range []string{
		`DROP TRIGGER IF EXISTS agent_unknown_disposition_no_update`,
		`DROP TRIGGER IF EXISTS agent_unknown_disposition_no_delete`,
		`DROP TABLE IF EXISTS agent_unknown_dispositions`,
		`DELETE FROM agent_schema_migrations WHERE version=16`,
		`DROP TRIGGER IF EXISTS agent_continuation_no_update`,
		`DROP TRIGGER IF EXISTS agent_continuation_no_delete`,
		`DROP TABLE IF EXISTS agent_task_continuations`,
		`DELETE FROM agent_schema_migrations WHERE version=15`,
		`DROP TRIGGER IF EXISTS agent_execution_event_no_delete`,
		`DROP TRIGGER IF EXISTS agent_execution_event_no_update`,
		`DROP TRIGGER IF EXISTS agent_native_session_no_delete`,
		`DROP TRIGGER IF EXISTS agent_native_session_no_update`,
		`DROP TABLE IF EXISTS agent_execution_events`,
		`DROP TABLE IF EXISTS agent_execution_processes`,
		`DROP TABLE IF EXISTS agent_execution_attempts`,
		`DROP TABLE IF EXISTS agent_native_sessions`,
		`DELETE FROM agent_schema_migrations WHERE version=14`,
		`CREATE TABLE agents_v7 (
    agent_id TEXT PRIMARY KEY, name TEXT NOT NULL, backend TEXT NOT NULL,
    plugin_id TEXT NOT NULL, extension_id TEXT NOT NULL,
    workspace_root TEXT NOT NULL, isolation TEXT NOT NULL,
    owner_kind TEXT NOT NULL, owner_ref TEXT NOT NULL, created_at TEXT NOT NULL,
    default_model TEXT NOT NULL DEFAULT '', default_reasoning_effort TEXT NOT NULL DEFAULT '',
    CHECK(backend IN ('codex','grok')),
    CHECK(isolation IN ('host','restricted_host'))
   )`,
		`INSERT INTO agents_v7 SELECT * FROM agents`,
		`DROP TABLE agents`,
		`ALTER TABLE agents_v7 RENAME TO agents`,
		`CREATE TRIGGER agent_bound_provider_delete_guard BEFORE DELETE ON agents
    WHEN EXISTS (SELECT 1 FROM provider_configs p WHERE p.plugin_id='io.mender.provider.agent-delegation'
    AND (json_valid(p.config_json)=0 OR json_extract(p.config_json,'$.agentId')=OLD.agent_id))
    BEGIN SELECT RAISE(ABORT,'Agent is referenced by a Provider'); END`,
		`DELETE FROM agent_schema_migrations WHERE version=8`,
		`DELETE FROM agent_schema_migrations WHERE version=13`,
		`DELETE FROM agent_schema_migrations WHERE version=12`,
		`DROP TRIGGER IF EXISTS agent_workspace_approval_policy_unbind`,
		`DROP TABLE agent_approval_policies`,
		`DROP TABLE agent_approval_policy_events`,
		`DELETE FROM agent_schema_migrations WHERE version=11`,
		`DROP TRIGGER IF EXISTS agent_placement_registered_workspace_delete_guard`,
		`DROP TABLE agent_tool_placements`,
		`DELETE FROM agent_schema_migrations WHERE version=10`,
		`DROP TABLE agent_task_approvals`,
		`DELETE FROM agent_schema_migrations WHERE version=9`,
		`ALTER TABLE agent_workspaces RENAME COLUMN workspace_root TO sandbox_id`,
	} {
		if _, err := tx.ExecContext(ctx, sqlText); err != nil {
			t.Fatalf("synthetic downgrade: %s: %v", sqlText, err)
		}
	}
	if len(sandboxID) > 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE agent_workspaces SET sandbox_id=? WHERE workspace_key='secondary'", sandboxID[0]); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
func TestOpenCodeAgentV7ToV8PreservesAllLiveRelationships(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "phase41.db")
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: dbPath, AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	owner := agentOwnerContext(t, r)
	agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{Name: "Existing Grok", Backend: agentdomain.BackendGrok,
		WorkspaceRoot: t.TempDir(), DefaultModel: "grok-4.7", DefaultReasoningEffort: "high"}, owner)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "migration-profile", DisplayName: "Migration profile", Mode: profiledomain.ExposureDirect})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	tools, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || tools.ProviderID == "" || len(tools.Tools) != 5 {
		t.Fatalf("facade not ready %+v %v", tools, err)
	}
	var delegateID string
	for _, tool := range tools.Tools {
		if tool.SourceKey == "agent.delegate" {
			delegateID = tool.ToolID
		}
	}
	if delegateID == "" {
		t.Fatal("missing Agent delegation Tool")
	}
	// Exact Profile ToolBinding must survive a rebuilt Agent table.
	// Tool IDs reference the existing Provider, never a copy of the Agent.
	// Workspace binding and grant survive even if no sandbox process is started.
	if _, err := r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: profile.Slug(), ToolID: mustToolID(t, delegateID), Direct: true}); err != nil {
		t.Fatal(err)
	}
	linkedRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacySandbox, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "Linked local workspace fixture", Root: linkedRoot, Isolation: sandboxdomain.IsolationHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.agentStore.BindWorkspace(ctx, agent.ID(), "secondary", linkedRoot); err != nil {
		t.Fatal(err)
	}
	if err := r.agentStore.GrantWorkspace(ctx, agent.ID(), "secondary", profile.ID().String()); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	history := agentdomain.Task{ID: taskID, AgentID: agent.ID(), Status: agentdomain.TaskQueued, Model: "grok-4.7", ReasoningEffort: "high",
		WorkspaceRoot: agent.Spec().WorkspaceRoot, Origin: "mcp", PrincipalKind: "service", PrincipalRef: "profile:fixture",
		ScopeKind: "profile", ScopeRef: "fixture", Generation: 1, CreatedAt: now, UpdatedAt: now}
	if err := r.agentStore.CreateTask(ctx, history); err != nil {
		t.Fatal(err)
	}
	if err := r.agentStore.Transition(ctx, taskID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.agentStore.Transition(ctx, taskID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "done", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeAgentsToV7Fixture(t, dbPath, legacySandbox.ID().String())
	// The migration is idempotent after one startup, changes only the CHECK,
	// and keeps all FK relationships and active Provider guards intact.
	r, err = NewRuntime(ctx, RuntimeOptions{DatabasePath: dbPath, AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	upgraded, err := r.Agents.Get(ctx, agent.ID(), owner)
	if err != nil || upgraded.Spec().DefaultModel != "grok-4.7" || upgraded.Spec().DefaultReasoningEffort != "high" {
		t.Fatalf("lost Agent defaults %+v %v", upgraded.Spec(), err)
	}
	result, err := r.Agents.Task(ctx, taskID, owner)
	if err != nil || result.Status != agentdomain.TaskCompleted || result.Result != "done" ||
		result.Model != "grok-4.7" || result.ReasoningEffort != "high" {
		t.Fatalf("lost Task history %+v %v", result, err)
	}
	grants, err := r.Agents.Grants(ctx, agent.ID(), owner)
	if err != nil || len(grants) != 1 || grants[0] != profile.ID().String() {
		t.Fatalf("lost AgentGrant %+v %v", grants, err)
	}
	workspaces, err := r.Agents.Workspaces(ctx, agent.ID(), owner)
	if err != nil || len(workspaces) != 1 || workspaces[0].Key != "secondary" || workspaces[0].WorkspaceRoot != linkedRoot || len(workspaces[0].ProfileIDs) != 1 {
		t.Fatalf("lost workspace and its Profile grant %+v %v", workspaces, err)
	}
	after, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || after.ProviderID != tools.ProviderID || len(after.Tools) != len(tools.Tools) {
		t.Fatalf("replaced Agent facade %+v %v", after, err)
	}
	for i, item := range tools.Tools {
		if item.ToolID != after.Tools[i].ToolID {
			t.Fatalf("rotated ToolID %s", item.ToolID)
		}
	}
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var profileToolBindings int
	if err := raw.QueryRowContext(ctx, "SELECT COUNT(*) FROM profile_tools WHERE tool_id=?", delegateID).Scan(&profileToolBindings); err != nil || profileToolBindings != 1 {
		t.Fatalf("lost Profile ToolBinding %d %v", profileToolBindings, err)
	}
	var version int
	if err := raw.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 16 {
		t.Fatalf("expected latest Agent v13 got %d %v", version, err)
	}
	checkRows, err := raw.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if checkRows.Next() {
		_ = checkRows.Close()
		t.Fatal("Agent v8 migrated broken foreign keys")
	}
	if err := checkRows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = checkRows.Close()
	if _, err := raw.ExecContext(ctx, "DELETE FROM agents WHERE agent_id=?", string(agent.ID())); err == nil || !strings.Contains(err.Error(), "Agent is referenced by a Provider") {
		t.Fatalf("guard lost or unexpectedly allowed delete: %v", err)
	}
	// A new OpenCode Agent uses the exact new built-in binding, not a cloned
	// Grok Provider or a second execution/governance path.
	created, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{Name: "OpenCode fixture", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: t.TempDir()}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if created.Spec().PluginID != agentdomain.OpenCodePluginID {
		t.Fatalf("wrong plugin %+v", created.Spec())
	}
	opencodeTools, err := r.AgentTools(ctx, created.ID(), owner)
	if err != nil || len(opencodeTools.Tools) != 5 || opencodeTools.ProviderID == tools.ProviderID {
		t.Fatalf("OpenCode suite not independent of existing identity %+v %v", opencodeTools, err)
	}
}

// Use the typed ToolID parser at the policy boundary.
func mustToolID(t *testing.T, id string) catalogdomain.ToolID {
	t.Helper()
	parsed, err := catalogdomain.ParseToolID(id)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// An unknown Agent trigger cannot be silently dropped by the v8 table
// replacement. The migration aborts before any destructive DDL.
func TestOpenCodeV8MigrationRejectsUnexpectedTrigger(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "unknown-trigger.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path, AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeAgentsToV7Fixture(t, path)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TRIGGER unknown_agent_policy BEFORE UPDATE ON agents
 BEGIN SELECT RAISE(ABORT,'unreviewed safety hook'); END`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	upgraded, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path, AgentExecutor: fixtureAgentExecutor{}})
	if err == nil {
		_ = upgraded.Close()
		t.Fatal("v8 migration discarded unreviewed Agent trigger")
	}
	if !strings.Contains(err.Error(), "refuses unexpected trigger") {
		t.Fatalf("failed for wrong reason: %v", err)
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var version int
	if err := raw.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 7 {
		t.Fatalf("failed migration modified schema version %d %v", version, err)
	}
	var guards int
	if err := raw.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='unknown_agent_policy'").Scan(&guards); err != nil || guards != 1 {
		t.Fatalf("failed migration lost unreviewed guard %d %v", guards, err)
	}
}
