package bootstrap

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agent "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentstore "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	v2fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v2"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	_ "modernc.org/sqlite"
)

// These tests start the SAME official NewRuntime used by menderd/Wails. They
// never construct a test-only typed Invocation, Sandbox Router, or MCP server.
func freshOfficialV3(t *testing.T) (*Runtime, *sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fresh-v3.sqlite")
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("close official runtime: %v", err)
		}
	})
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var sourceCount, legacyTables int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tools WHERE source_kind='native'").Scan(&sourceCount); err != nil || sourceCount != 9 {
		t.Fatalf("fresh canonical Tools: %d %v", sourceCount, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
 WHERE type='table' AND name IN ('agent_tool_facades','sandbox_tool_suite_bindings',
 'sandbox_job_suite_bindings','workspace_tool_targets')`).Scan(&legacyTables); err != nil || legacyTables != 0 {
		t.Fatalf("new runtime revived old facade tables: %d %v", legacyTables, err)
	}
	return r, db, path
}

func officialMCPClient(t *testing.T, r *Runtime, pSlug string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	slug, err := profiledomain.NewSlug(pSlug)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.Profiles.Get.Execute(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := r.NewMCPServer(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	server, err := adapter.MCPServer().Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "official-v3-runtime-acceptance", Version: "1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func nativeID(t *testing.T, key string) catalog.ToolID {
	t.Helper()
	items, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.SourceKey == key {
			return item.ToolID
		}
	}
	t.Fatalf("native tool %s not found", key)
	return ""
}

func TestOfficialV3RuntimeFreshWorkspaceMCPInvocationAndRevocation(t *testing.T) {
	ctx := context.Background()
	r, db, _ := freshOfficialV3(t)
	owner, err := governance.NewContext(governance.OriginCLI, r.OwnerPrincipal(), governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("official v3 native call\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sb, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{Name: "Official F4", Root: root, Isolation: sandbox.IsolationHost}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := r.SandboxWorkspaceTools(ctx, sb.ID(), owner)
	if err != nil || len(suite.Tools) != 4 {
		t.Fatalf("Sandbox native tool projection: %+v %v", suite, err)
	}
	p, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "native-v3-test", DisplayName: "Native V3"})
	if err != nil {
		t.Fatal(err)
	}
	id := nativeID(t, "workspace.read")
	if _, err := r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: p.Slug(), ToolID: id}); err != nil {
		t.Fatal(err)
	}
	workspaceID := "wrk_11111111111111111111111111111111"
	for _, record := range []struct {
		stmt string
		args []any
	}{
		{`INSERT INTO workspaces(workspace_id,owner_kind,owner_ref,display_name,verified_root,created_at)
 VALUES(?,?,?,?,?,?)`, []any{workspaceID, string(owner.Principal().Kind()), owner.Principal().Reference(), "F4 test", root, time.Now().UTC().Format(time.RFC3339Nano)}},
		{"INSERT INTO workspace_placements(workspace_id,sandbox_id,backend_root,verified_revision) VALUES(?,?,?,1)", []any{workspaceID, sb.ID().String(), root}},
		{"INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision) VALUES(?,'direct','',0)", []any{p.ID().String()}},
		{"INSERT INTO mcp_delivery_direct_tools(profile_id,tool_id) VALUES(?,?)", []any{p.ID().String(), id.String()}},
		{"INSERT INTO sandbox_profile_grants(sandbox_id,profile_id,capability,granted_at) VALUES(?,?,'file.read','test')", []any{sb.ID().String(), p.ID().String()}},
		{"INSERT INTO workspace_profile_grants(workspace_id,profile_id,granted_at) VALUES(?,?,'test')", []any{workspaceID, p.ID().String()}},
		{"INSERT INTO profile_tool_target_scopes(profile_id,tool_id,workspace_id,sandbox_id) VALUES(?,?,?,?)", []any{p.ID().String(), id.String(), workspaceID, sb.ID().String()}},
	} {
		if _, err := db.ExecContext(ctx, record.stmt, record.args...); err != nil {
			t.Fatalf("explicit authorized test fixture: %v", err)
		}
	}
	client := officialMCPClient(t, r, "native-v3-test")
	tools, err := client.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 1 {
		t.Fatalf("official Native MCP export: %+v %v", tools, err)
	}
	args := map[string]any{"workspaceId": workspaceID, "path": "README.md"}
	call, err := client.CallTool(ctx, &mcp.CallToolParams{Name: tools.Tools[0].Name, Arguments: args})
	if err != nil || call.IsError {
		t.Fatalf("official Native MCP call: %+v %v", call, err)
	}
	body, ok := call.StructuredContent.(map[string]any)
	if !ok || body["content"] != "official v3 native call" {
		t.Fatalf("unexpected native result: %+v", call.StructuredContent)
	}
	var leases int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_execution_leases").Scan(&leases); err != nil || leases != 0 {
		t.Fatalf("Native lease leaked: %d %v", leases, err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM workspace_profile_grants WHERE workspace_id=? AND profile_id=?", workspaceID, p.ID().String()); err != nil {
		t.Fatal(err)
	}
	denied, err := client.CallTool(ctx, &mcp.CallToolParams{Name: tools.Tools[0].Name, Arguments: args})
	if err == nil && denied != nil && !denied.IsError {
		t.Fatal("revoked Workspace still executable via stale MCP export")
	}
}

func TestOfficialV3RuntimeAgentToolStatusAndRevocation(t *testing.T) {
	ctx := context.Background()
	r, db, path := freshOfficialV3(t)
	owner, err := governance.NewContext(governance.OriginCLI, r.OwnerPrincipal(), governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	created, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Official Agent", Backend: agent.BackendGrok, WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := r.AgentTools(ctx, created.ID(), owner)
	if err != nil || suite.Status != "ready" || len(suite.Tools) != 5 {
		t.Fatalf("Agent owned source: %+v %v", suite, err)
	}
	var statusID catalog.ToolID
	for _, tool := range suite.Tools {
		if tool.SourceKey == "agent.status" {
			statusID, err = catalog.ParseToolID(tool.ToolID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	p, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "agent-v3-test", DisplayName: "Agent V3"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: p.Slug(), ToolID: statusID}); err != nil {
		t.Fatal(err)
	}
	if err := r.Agents.Grant(ctx, created.ID(), p.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision) VALUES(?,'direct','',0)", p.ID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO mcp_delivery_direct_tools(profile_id,tool_id) VALUES(?,?)", p.ID().String(), statusID.String()); err != nil {
		t.Fatal(err)
	}
	store, err := agentstore.OpenTyped(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	taskID, err := agent.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := agent.Task{ID: taskID, AgentID: created.ID(), Status: agent.TaskQueued,
		Origin: "mcp", PrincipalKind: string(governance.PrincipalService),
		PrincipalRef: "profile:" + p.ID().String(), ScopeKind: string(governance.ScopeProfile),
		ScopeRef: p.ID().String(), Generation: 1, TimeoutSeconds: 600,
		CreatedAt: now, UpdatedAt: now}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	client := officialMCPClient(t, r, "agent-v3-test")
	list, err := client.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 1 {
		t.Fatalf("Agent source MCP export: %+v %v", list, err)
	}
	args := map[string]any{"taskId": string(taskID)}
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: list.Tools[0].Name, Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("Agent Task status: %+v %v", result, err)
	}
	body, ok := result.StructuredContent.(map[string]any)
	if !ok || body["status"] != "queued" {
		t.Fatalf("incorrect Agent status: %+v", result.StructuredContent)
	}
	if err := r.Agents.Revoke(ctx, created.ID(), p.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	denied, err := client.CallTool(ctx, &mcp.CallToolParams{Name: list.Tools[0].Name, Arguments: args})
	if err == nil && denied != nil && !denied.IsError {
		t.Fatal("revoked Agent Task status still accessible")
	}
}

// Native SSH Job source is globally catalogued, but it must never submit,
// observe or cancel any Job without an exact TargetScope and Sandbox grant.
func TestOfficialV3NativeJobsFailClosedWithoutTargetScope(t *testing.T) {
	ctx := context.Background()
	r, db, _ := freshOfficialV3(t)
	p, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "native-jobs-test", DisplayName: "Native Jobs"})
	if err != nil {
		t.Fatal(err)
	}
	id := nativeID(t, "jobs.status")
	if _, err := r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: p.Slug(), ToolID: id}); err != nil {
		t.Fatal(err)
	}
	principal, err := governance.NewPrincipal(governance.PrincipalService, "profile:"+p.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	scope, err := governance.ProfileScope(p.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	caller, err := governance.NewContext(governance.OriginMCP, principal, scope)
	if err != nil {
		t.Fatal(err)
	}
	response, err := r.InvokeTool.Execute(ctx, invocationapp.InvokeToolRequest{
		ToolID: id, Context: caller, Arguments: []byte(`{"sandboxId":"sbx_11111111111111111111111111111111","taskId":"sjob_11111111111111111111111111111111"}`),
	})
	if err == nil && response.Outcome.Kind() != invocation.OutcomeKindRejected && response.Outcome.Kind() != invocation.OutcomeKindFailed {
		t.Fatalf("ungranted Native SSH Job operation executed: %+v", response)
	}
	var admissions int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_admissions WHERE status='admitted'").Scan(&admissions); err != nil || admissions != 0 {
		t.Fatalf("unauthorized Job acquired Sandbox: %d %v", admissions, err)
	}
}

func TestOfficialV3RejectsPreexistingNonV3DatabaseWithoutMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "unapproved-v2.sqlite")
	original := []byte("this is a pre-existing unapproved database")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path}); err == nil {
		_ = r.Close()
		t.Fatal("v3 runtime silently migrated pre-existing data")
	}
	// #nosec G304 -- path is a test-owned temporary database filename.
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(original) {
		t.Fatalf("pre-existing database modified: %v", err)
	}
}

// Official external MCP Provider execution and retirement must use the same
// typed Catalog and canonical Invocation; no resource Provider facade exists.
func TestOfficialV3ExternalMCPProviderAndTransactionalRetirement(t *testing.T) {
	ctx := context.Background()
	r, db, _ := freshOfficialV3(t)
	upstream := mcp.NewServer(&mcp.Implementation{Name: "f4-provider", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "echo_test", Title: "Echo",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{StructuredContent: map[string]any{"message": "f4-provider-ok"}}, nil
		})
	remote := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return upstream },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	t.Cleanup(remote.Close)
	raw, err := json.Marshal(mcpplugin.Config{Transport: mcpplugin.TransportConfig{
		Kind: mcpplugin.TransportStreamableHTTP, Endpoint: remote.URL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, r, ctx,
		catalogapp.RegisterProviderRequest{Namespace: "f4-external-provider", DisplayName: "F4 External"}, mcpplugin.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	result, err := syncConfiguredProviderTest(t, r, ctx, provider.ID())
	if err != nil || result.Total != 1 {
		t.Fatalf("external Provider sync %+v %v", result, err)
	}
	tools, err := r.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil || len(tools) != 1 || tools[0].SourceRef().Kind() != catalog.SourceProvider {
		t.Fatalf("typed Provider Tools %+v %v", tools, err)
	}
	p, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "f4-external-v3", DisplayName: "F4 External"})
	if err != nil {
		t.Fatal(err)
	}
	p, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: p.Slug(), ToolID: tools[0].ID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision) VALUES(?,'direct','',0)", p.ID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO mcp_delivery_direct_tools(profile_id,tool_id) VALUES(?,?)", p.ID().String(), tools[0].ID().String()); err != nil {
		t.Fatal(err)
	}
	client := officialMCPClient(t, r, "f4-external-v3")
	list, err := client.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 1 {
		t.Fatalf("external MCP Tool list %+v %v", list, err)
	}
	resp, err := client.CallTool(ctx, &mcp.CallToolParams{Name: list.Tools[0].Name, Arguments: map[string]any{}})
	if err != nil || resp.IsError {
		t.Fatalf("external invocation %+v %v", resp, err)
	}
	output, ok := resp.StructuredContent.(map[string]any)
	if !ok || output["message"] != "f4-provider-ok" {
		t.Fatalf("external MCP outcome %+v", resp.StructuredContent)
	}
	if err = r.DeleteConfiguredProvider(ctx, provider.ID()); err == nil {
		t.Fatal("Provider with bound ToolID retired despite Profile reference")
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM providers WHERE id=?", provider.ID().String()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed retirement deleted Provider: %d %v", count, err)
	}
	// The Profile's direct export and ToolBinding are removed explicitly in
	// this disposable fixture; no automatic permission repair is allowed.
	if _, err = db.ExecContext(ctx, "DELETE FROM mcp_delivery_direct_tools WHERE profile_id=? AND tool_id=?", p.ID().String(), tools[0].ID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "DELETE FROM profile_tools WHERE profile_id=? AND tool_id=?", p.ID().String(), tools[0].ID().String()); err != nil {
		t.Fatal(err)
	}
	if err = r.DeleteConfiguredProvider(ctx, provider.ID()); err != nil {
		t.Fatalf("retire now-unbound typed Provider: %v", err)
	}
	for name, stmt := range map[string]string{
		"Provider": "SELECT COUNT(*) FROM providers WHERE id=?",
		"Config":   "SELECT COUNT(*) FROM provider_configs WHERE provider_id=?",
		"Tool":     "SELECT COUNT(*) FROM tools WHERE source_kind='provider' AND source_id=?",
		"FTS":      "SELECT COUNT(*) FROM tools_fts WHERE source_kind='provider' AND source_id=?",
	} {
		if err = db.QueryRowContext(ctx, stmt, provider.ID().String()).Scan(&count); err != nil || count != 0 {
			t.Fatalf("retired Provider left %s rows: %d %v", name, count, err)
		}
	}
}

// Sandbox control operations do not require Tool Invocation receipts. A
// logical Destroy must nevertheless preserve an extant Workspace placement.
func TestOfficialV3SandboxLifecycleUsesResourceLeaseAndProtectsWorkspacePlacement(t *testing.T) {
	ctx := context.Background()
	r, db, _ := freshOfficialV3(t)
	owner, err := governance.NewContext(governance.OriginCLI, r.OwnerPrincipal(), governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	item, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "v3 lifecycle", Root: root, Isolation: sandbox.IsolationHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governance.ActionSandboxStop, owner); err != nil {
		t.Fatalf("Owner stop required a Tool invocation receipt: %v", err)
	}
	if _, err = r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governance.ActionSandboxStart, owner); err != nil {
		t.Fatalf("Owner restart required a Tool invocation receipt: %v", err)
	}
	ws := "wrk_22222222222222222222222222222222"
	if _, err = db.ExecContext(ctx, `INSERT INTO workspaces(
		workspace_id,owner_kind,owner_ref,display_name,verified_root,created_at)
		VALUES(?,?,?,?,?,?)`, ws, string(owner.Principal().Kind()),
		owner.Principal().Reference(), "pinned", root, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO workspace_placements(
		workspace_id,sandbox_id,backend_root,verified_revision) VALUES(?,?,?,3)`, ws, item.ID().String(), root); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governance.ActionSandboxDestroy, owner); !errors.Is(err, sandbox.ErrConflict) {
		t.Fatalf("Destroy invalidated pinned Workspace: %v", err)
	}
	if _, err = db.ExecContext(ctx, "DELETE FROM workspace_placements WHERE workspace_id=? AND sandbox_id=?", ws, item.ID().String()); err != nil {
		t.Fatal(err)
	}
	destroyed, err := r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governance.ActionSandboxDestroy, owner)
	if err != nil || destroyed.State != sandbox.StateDestroyed {
		t.Fatalf("Unbound Sandbox cannot be destroyed: %+v %v", destroyed, err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("Logical Sandbox Destroy modified host Workspace: %v", err)
	}
}

// Restart uses the same official v3 runtime and must preserve Agent as Tool
// IDs without minting an Agent Provider or losing the original Agent resource.
func TestOfficialV3RestartRetainsAgentSourceIdentity(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "restart-v3.sqlite")
	first, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := governance.NewContext(governance.OriginCLI, first.OwnerPrincipal(), governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	created, err := first.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Official persistent Agent", Backend: agent.BackendGrok, WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	before, err := first.AgentTools(ctx, created.ID(), owner)
	if err != nil || before.Status != "ready" || len(before.Tools) != 5 {
		t.Fatalf("before restart %+v %v", before, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	after, err := reopened.AgentTools(ctx, created.ID(), owner)
	if err != nil || after.Status != "ready" {
		t.Fatalf("after restart %+v %v", after, err)
	}
	if !reflect.DeepEqual(before.Tools, after.Tools) {
		t.Fatalf("Agent Tool identities rotated: before=%+v after=%+v", before.Tools, after.Tools)
	}
	// A second explicit ensure is an idempotent metadata refresh, not a new tool.
	verified, err := reopened.EnsureAgentTools(ctx, created.ID(), owner)
	if err != nil || len(verified.Tools) != 5 || !reflect.DeepEqual(after.Tools, verified.Tools) {
		t.Fatalf("repeated owner supply changed ToolIDs %+v %v", verified, err)
	}
}

// A previously valid v2 database, unlike a new absent path, must remain
// byte-for-byte untouched when the official v3 entry refuses to open it.
func TestOfficialV3RejectsExistingLegacyDatabaseWithoutWrites(t *testing.T) {
	ctx := context.Background()
	path := v2fixture.EmptyDatabase(t)
	// #nosec G304 -- path comes from the disposable historical fixture.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if live, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path}); err == nil {
		_ = live.Close()
		t.Fatal("official v3 runtime unexpectedly started on old Provider-v2 data")
	}
	// #nosec G304 -- only the test-created v2 file is read here.
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("v3 altered historical v2 input: %v", err)
	}
}
