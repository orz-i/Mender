package typedsupply_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	"github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	v2fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v2"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	_ "modernc.org/sqlite"
)

const profileA = "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const profileB = "prof_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

type futureProfileFixture struct {
	path         string
	sandboxID    string
	workspaceID  string
	agentID      string
	agentToolIDs []string
	nativeReadID string
	db           *sql.DB
}

func futureFixture(t *testing.T) *futureProfileFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v3.sqlite")
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := gov.NewContext(gov.OriginCLI, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sb, err := runtime.Sandboxes.CreateLocalSandbox(ctx, bootstrap.CreateLocalSandboxRequest{
		Name: "Profile scope source", Root: root, Isolation: sandbox.IsolationHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := runtime.SandboxWorkspaceTools(ctx, sb.ID(), owner)
	if err != nil || len(suite.Tools) != 4 {
		t.Fatalf("Suite %v %+v", err, suite)
	}
	agent, err := runtime.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Code reviewer", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	agentSuite, err := runtime.AgentTools(ctx, agent.ID(), owner)
	if err != nil || len(agentSuite.Tools) != 5 {
		t.Fatalf("Agent as Tool %v %+v", err, agentSuite)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	workspaceID := "wrk_33333333333333333333333333333333"
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspaces(workspace_id,owner_kind,owner_ref,display_name,verified_root,created_at) VALUES(?,?,?,?,?,?)`, []any{workspaceID, string(owner.Principal().Kind()), owner.Principal().Reference(), "Test Workspace", root, time.Now().UTC().Format(time.RFC3339Nano)}},
		{`INSERT INTO workspace_placements(workspace_id,sandbox_id,backend_root,verified_revision) VALUES(?,?,?,1)`, []any{workspaceID, sb.ID().String(), root}},
	} {
		if _, err := db.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
	tools, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	var nativeReadID string
	for _, item := range tools {
		if item.SourceKey == "workspace.read" {
			nativeReadID = item.ToolID.String()
		}
	}
	if nativeReadID == "" {
		t.Fatal("missing host reviewed native read ToolID")
	}
	fixture := &futureProfileFixture{path: path,
		sandboxID: sb.ID().String(), workspaceID: workspaceID, agentID: string(agent.ID()), nativeReadID: nativeReadID, db: db}
	for _, item := range agentSuite.Tools {
		fixture.agentToolIDs = append(fixture.agentToolIDs, item.ToolID)
	}
	return fixture
}
func (f *futureProfileFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		t.Fatalf("fixture SQL: %v: %s", err, query)
	}
}
func (f *futureProfileFixture) addProfile(t *testing.T, id, slug, mode string) {
	t.Helper()
	f.exec(t, `INSERT INTO profiles(id,slug,display_name,revision) VALUES(?,?,?,0)`, id, slug, slug)
	f.exec(t, `INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision) VALUES(?,?,'',0)`, id, mode)
}
func (f *futureProfileFixture) addTool(t *testing.T, profileID, toolID, alias string) {
	t.Helper()
	f.exec(t, `INSERT INTO profile_tools
 (profile_id,tool_id,alias,title_override,description_override,policy_constraint,concurrency_constraint,credential_refs_json)
 VALUES(?,?,?,'','','inherit','inherit','[]')`, profileID, toolID, alias)
}
func (f *futureProfileFixture) grantAgent(t *testing.T, profileID string) {
	t.Helper()
	f.exec(t, `INSERT INTO agent_grants(agent_id,profile_id,created_at) VALUES(?,?,'now')`, f.agentID, profileID)
}
func (f *futureProfileFixture) grantNative(t *testing.T, profileID string) {
	t.Helper()
	f.exec(t, `INSERT INTO sandbox_profile_grants(sandbox_id,profile_id,capability,granted_at)
 VALUES(?,?,'file.read','now')`, f.sandboxID, profileID)
	f.exec(t, `INSERT INTO workspace_profile_grants(workspace_id,profile_id,granted_at)
 VALUES(?,?,'now')`, f.workspaceID, profileID)
	f.exec(t, `INSERT INTO profile_tool_target_scopes(profile_id,tool_id,workspace_id,sandbox_id)
 VALUES(?,?,?,?)`, profileID, f.nativeReadID, f.workspaceID, f.sandboxID)
}
func TestTypedProfileSupplyReadOnlyAndRevocation(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "main", "direct")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	f.addTool(t, profileA, f.nativeReadID, "read")
	f.grantAgent(t, profileA)
	f.grantNative(t, profileA)
	reader, err := typedsupply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	got, err := reader.Project(context.Background(), profileA)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 || len(got.Direct) != 2 {
		t.Fatalf("expected Agent+Native Profile sources, got %+v", got)
	}
	if got.Sources[0].SourceKind != "agent" || got.Sources[0].SourceID != "agent:"+f.agentID ||
		got.Sources[1].SourceID != "native:workspace" {
		t.Fatalf("real provenance lost: %+v", got.Sources)
	}
	for _, export := range got.Direct {
		if strings.HasPrefix(export.Name, "sandbox-") || strings.HasPrefix(export.Name, "prov_") {
			t.Fatalf("export kept managed Provider source: %+v", export)
		}
	}
	target := got.Sources[1].Tools[0].Targets
	if len(target) != 1 || target[0].WorkspaceID != f.workspaceID || target[0].SandboxID != f.sandboxID {
		t.Fatalf("native target expanded: %+v", target)
	}
	// Exact Profile grants and target scopes must be freshly loaded, not
	// inherited from an unrelated Sandbox Grant or cached Tool list.
	f.exec(t, "DELETE FROM profile_tool_target_scopes WHERE profile_id=?", profileA)
	next, err := reader.Project(context.Background(), profileA)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Sources) != 1 || next.Sources[0].SourceKind != "agent" {
		t.Fatalf("removed target scope was still exposed: %+v", next)
	}
	f.exec(t, "DELETE FROM agent_grants WHERE profile_id=?", profileA)
	empty, err := reader.Project(context.Background(), profileA)
	if err != nil || len(empty.Sources) != 0 || len(empty.Direct) != 0 {
		t.Fatalf("Agent grant revocation leaked tool: %+v %v", empty, err)
	}
}
func TestHybridAndDynamicExportSelectionIsExact(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "hybrid", "hybrid")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	f.addTool(t, profileA, f.nativeReadID, "read")
	f.grantAgent(t, profileA)
	f.grantNative(t, profileA)
	f.exec(t, `INSERT INTO mcp_delivery_direct_tools(profile_id,tool_id)
 VALUES(?,?)`, profileA, f.nativeReadID)
	reader, err := typedsupply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	hybrid, err := reader.Project(context.Background(), profileA)
	if err != nil {
		t.Fatal(err)
	}
	if len(hybrid.Sources) != 2 || len(hybrid.Direct) != 1 ||
		hybrid.Direct[0].ToolID != f.nativeReadID ||
		hybrid.Direct[0].Name != "workspace__read" {
		t.Fatalf("Hybrid selection ignored explicit Direct subset: %+v", hybrid)
	}
	f.addProfile(t, profileB, "dynamic", "dynamic")
	f.addTool(t, profileB, f.nativeReadID, "read")
	f.grantNative(t, profileB)
	dynamic, err := reader.Project(context.Background(), profileB)
	if err != nil || len(dynamic.Sources) != 1 || len(dynamic.Direct) != 0 {
		t.Fatalf("Dynamic mode emitted unexpected direct tools: %+v %v", dynamic, err)
	}
}
func TestTypedProfileNoCrossProfileSourceDisclosure(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "visible", "direct")
	f.addProfile(t, profileB, "hidden", "direct")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	f.addTool(t, profileB, f.agentToolIDs[0], "delegate")
	f.grantAgent(t, profileA)
	reader, err := typedsupply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	visible, err := reader.Project(context.Background(), profileA)
	if err != nil || len(visible.Sources) != 1 {
		t.Fatal("authorized Agent missing", err)
	}
	hidden, err := reader.Project(context.Background(), profileB)
	if err != nil || len(hidden.Sources) != 0 || len(hidden.Direct) != 0 {
		t.Fatalf("Profile B discovered another Profile's Agent: %+v %v", hidden, err)
	}
	if _, err := reader.Project(context.Background(), "prof_invalid"); !errors.Is(err, typedsupply.ErrInvalidSupply) {
		t.Fatalf("invalid Profile accepted: %v", err)
	}
	if _, err := reader.Project(context.Background(), "prof_11111111111111111111111111111111"); !errors.Is(err, typedsupply.ErrInvalidSupply) {
		t.Fatalf("unknown Profile disclosed source: %v", err)
	}
}
func TestTypedSupplyRejectsLegacySchemaWithoutMigration(t *testing.T) {
	path := v2fixture.EmptyDatabase(t)
	reader, err := typedsupply.Open(context.Background(), path)
	if reader != nil {
		_ = reader.Close()
	}
	if !errors.Is(err, typedsupply.ErrTypedSchemaRequired) {
		t.Fatalf("legacy Provider schema accepted: %v", err)
	}
}
func TestDirectExportCollisionFailsClosed(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "colliding", "direct")
	if len(f.agentToolIDs) < 2 {
		t.Fatal("Agent Tool Suite lacks control actions")
	}
	f.addTool(t, profileA, f.agentToolIDs[0], "same")
	f.addTool(t, profileA, f.agentToolIDs[1], "same")
	f.grantAgent(t, profileA)
	reader, err := typedsupply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	if _, err := reader.Project(context.Background(), profileA); !errors.Is(err, typedsupply.ErrInvalidSupply) {
		t.Fatalf("different Agent Tools with same effective exported name accepted: %v", err)
	}
}
