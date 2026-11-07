package typedlease_test

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	migration "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/typedlease"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
	_ "modernc.org/sqlite"
)

const profileID = "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const invocationID = "inv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type receiptVerifier struct {
	deny   bool
	checks int
}

func (a *receiptVerifier) VerifyAdmittedAttempt(_ context.Context, id invocation.InvocationID,
	caller governance.Context, tool catalog.ToolID, facts invocation.GovernanceFacts) error {
	a.checks++
	if a.deny || id != invocationID || caller.Validate() != nil || tool == "" || facts.Validate() != nil {
		return errors.New("Invocation approval is absent or revoked")
	}
	return nil
}

type atomicFixture struct {
	db       *sql.DB
	store    *typedlease.Store
	verifier *receiptVerifier
	request  typedlease.Request
}

func atomicWorkspaceFixture(t *testing.T) *atomicFixture {
	t.Helper()
	ctx := context.Background()
	legacyPath := filepath.Join(t.TempDir(), "legacy.sqlite")
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: legacyPath})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := governance.NewContext(governance.OriginCLI, runtime.OwnerPrincipal(), governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	item, err := runtime.Sandboxes.CreateLocalSandbox(ctx, bootstrap.CreateLocalSandboxRequest{
		Name: "Scope integration", Root: root, Isolation: sandbox.IsolationHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EnsureSandboxWorkspaceTools(ctx, item.ID(), owner); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	plan, err := migration.AssessPath(ctx, legacyPath)
	if err != nil || plan.Status != "ready" {
		t.Fatalf("Stage E assessment not ready: %v %+v", err, plan.Conflicts)
	}
	candidate, err := migration.StageCopy(ctx, legacyPath, filepath.Join(t.TempDir(), "candidate.sqlite"), plan.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(candidate.CandidatePath)}
	q := uri.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	var wsRef string
	var generation uint64
	if err := db.QueryRow(`SELECT workspace_id,verified_revision FROM workspace_placements
   WHERE sandbox_id=?`, item.ID().String()).Scan(&wsRef, &generation); err != nil {
		t.Fatal(err)
	}
	defs, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	tool := catalog.ToolID("")
	for _, def := range defs {
		if def.SourceKey == "workspace.read" {
			tool = def.ToolID
			break
		}
	}
	if tool == "" {
		t.Fatal("native read identity not registered")
	}
	for _, cmd := range []struct {
		query  string
		params []any
	}{
		{`INSERT INTO profiles(id,slug,display_name,revision) VALUES(?,'scope-test','Scope Test',0)`, []any{profileID}},
		{`INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision) VALUES(?,'direct','',0)`, []any{profileID}},
		{`INSERT INTO profile_tools(profile_id,tool_id,alias,title_override,description_override,
      policy_constraint,concurrency_constraint,credential_refs_json)
    VALUES(?,?,'','','','inherit','inherit','[]')`, []any{profileID, tool.String()}},
		{`INSERT INTO sandbox_profile_grants(sandbox_id,profile_id,capability,granted_at)
    VALUES(?,?,'file.read','now')`, []any{item.ID().String(), profileID}},
		{`INSERT INTO workspace_profile_grants(workspace_id,profile_id,granted_at) VALUES(?,?,'now')`, []any{wsRef, profileID}},
		{`INSERT INTO profile_tool_target_scopes(profile_id,tool_id,workspace_id,sandbox_id)
    VALUES(?,?,?,?)`, []any{profileID, tool.String(), wsRef, item.ID().String()}},
	} {
		if _, err := db.Exec(cmd.query, cmd.params...); err != nil {
			t.Fatalf("fixture SQL: %v", err)
		}
	}
	principal, err := governance.NewPrincipal(governance.PrincipalService, "profile:"+profileID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := governance.ProfileScope(profileID)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := governance.NewContext(governance.OriginMCP, principal, scope)
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := workspace.ParseRef(wsRef)
	sandboxID, _ := sandbox.ParseID(item.ID().String())
	wf, err := invocation.NewResourceFact("workspace_id", ws.String())
	if err != nil {
		t.Fatal(err)
	}
	sf, err := invocation.NewResourceFact("sandbox_id", sandboxID.String())
	if err != nil {
		t.Fatal(err)
	}
	facts, err := invocation.KnownGovernanceFacts([]invocation.EffectFact{"filesystem.read"}, []invocation.ResourceFact{wf, sf})
	if err != nil {
		t.Fatal(err)
	}
	verifier := &receiptVerifier{}
	store, err := typedlease.New(ctx, db, verifier)
	if err != nil {
		t.Fatal(err)
	}
	request := typedlease.Request{
		InvocationID: invocationID, Caller: caller, ToolID: tool, Operation: workspace.Read,
		Workspace: ws, Sandbox: sandboxID, Generation: generation,
		Isolation: sandbox.IsolationHost, ApprovedFacts: facts,
	}
	return &atomicFixture{db: db, store: store, verifier: verifier, request: request}
}
func (f *atomicFixture) exec(t *testing.T, q string, params ...any) {
	t.Helper()
	if _, err := f.db.Exec(q, params...); err != nil {
		t.Fatal(err)
	}
}
func (f *atomicFixture) count(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM sandbox_execution_leases").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestAtomicNativeLeaseMatchesAllReviewedAuthorities(t *testing.T) {
	f := atomicWorkspaceFixture(t)
	ctx := context.Background()
	token, err := f.store.Reserve(ctx, f.request)
	if err != nil || len(token) != 32 || f.count(t) != 1 {
		t.Fatalf("exact approved native lease not admitted: %q %v", token, err)
	}
	if f.verifier.checks != 1 {
		t.Fatal("Invocation verifier was not checked")
	}
	if _, err := f.store.Reserve(ctx, f.request); err == nil || f.count(t) != 1 {
		t.Fatalf("same Sandbox acquired twice: %v", err)
	}
	if err := f.store.Release(ctx, f.request.Caller, f.request.Sandbox, "deadbeef"); err == nil {
		t.Fatal("invalid lease token released another execution")
	}
	if f.count(t) != 1 {
		t.Fatal("lease was removed by invalid request")
	}
	if err := f.store.Release(ctx, f.request.Caller, f.request.Sandbox, token); err != nil {
		t.Fatal(err)
	}
	if f.count(t) != 0 {
		t.Fatal("durable lease not released")
	}
}
func TestAtomicNativeLeaseCannotUseStaleScopeOrGrants(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*atomicFixture, *testing.T)
	}{
		{"Workspace grant revoked", func(f *atomicFixture, t *testing.T) {
			f.exec(t, "DELETE FROM workspace_profile_grants WHERE workspace_id=? AND profile_id=?", f.request.Workspace.String(), profileID)
		}},
		{"Sandbox capability revoked", func(f *atomicFixture, t *testing.T) {
			f.exec(t, "DELETE FROM sandbox_profile_grants WHERE sandbox_id=? AND profile_id=?", f.request.Sandbox.String(), profileID)
		}},
		{"ToolBinding scope revoked", func(f *atomicFixture, t *testing.T) {
			f.exec(t, "DELETE FROM profile_tool_target_scopes WHERE profile_id=?", profileID)
		}},
		{"Run generation changed", func(f *atomicFixture, t *testing.T) {
			f.exec(t, "UPDATE sandbox_runs SET generation=generation+1 WHERE sandbox_id=?", f.request.Sandbox.String())
		}},
		{"Run became stopped", func(f *atomicFixture, t *testing.T) {
			f.exec(t, "UPDATE sandbox_runs SET state='stopped' WHERE sandbox_id=?", f.request.Sandbox.String())
		}},
		{"Tool binding policy denies", func(f *atomicFixture, t *testing.T) {
			f.exec(t, "UPDATE profile_tools SET policy_constraint='deny' WHERE profile_id=?", profileID)
		}},
		{"Workspace root binding changed", func(f *atomicFixture, t *testing.T) {
			f.exec(t, "UPDATE sandboxes SET local_root='/different' WHERE id=?", f.request.Sandbox.String())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := atomicWorkspaceFixture(t)
			tc.change(f, t)
			if _, err := f.store.Reserve(context.Background(), f.request); err == nil {
				t.Fatal("revoked or stale lease was reserved")
			}
			if f.count(t) != 0 {
				t.Fatal("revocation still created durable lease")
			}
		})
	}
}
func TestAtomicLeaseRejectsUnapprovedInvocationAndDifferentTool(t *testing.T) {
	f := atomicWorkspaceFixture(t)
	f.verifier.deny = true
	if _, err := f.store.Reserve(context.Background(), f.request); !errors.Is(err, typedlease.ErrUnadmitted) {
		t.Fatalf("forged Approval receipt accepted: %v", err)
	}
	f.verifier.deny = false
	bad := f.request
	bad.ToolID = "tool_11111111111111111111111111111111"
	if _, err := f.store.Reserve(context.Background(), bad); err == nil {
		t.Fatal("Provider ToolID impersonated native read")
	}
	bad = f.request
	bad.Sandbox = "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := f.store.Reserve(context.Background(), bad); err == nil {
		t.Fatal("approved Sandbox A substituted for Sandbox B")
	}
	bad = f.request
	bad.Workspace = "wrk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := f.store.Reserve(context.Background(), bad); err == nil {
		t.Fatal("approved Workspace was replaced")
	}
	if f.count(t) != 0 {
		t.Fatal("failed admission left a lease")
	}
}
func TestTypedLeaseRejectsLegacySchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := typedlease.New(ctx, db, &receiptVerifier{}); err == nil {
		t.Fatal("typed lease accepted Provider-only legacy Schema")
	}
}

func TestAtomicNativeLeaseConcurrentGrantRevocationIsLinearizable(t *testing.T) {
	// A legitimate reservation may commit before revocation, but once the
	// independent Workspace grant has been revoked and committed no new
	// reservation may succeed. All writers share the same SQLite lease table.
	for attempt := 0; attempt < 8; attempt++ {
		f := atomicWorkspaceFixture(t)
		start := make(chan struct{})
		reserveDone := make(chan struct{})
		revokeDone := make(chan error, 1)
		var token string
		var reserveErr error
		go func() {
			<-start
			token, reserveErr = f.store.Reserve(context.Background(), f.request)
			close(reserveDone)
		}()
		go func() {
			<-start
			_, err := f.db.ExecContext(context.Background(),
				"DELETE FROM workspace_profile_grants WHERE workspace_id=? AND profile_id=?",
				f.request.Workspace.String(), profileID)
			revokeDone <- err
		}()
		close(start)
		<-reserveDone
		if err := <-revokeDone; err != nil {
			t.Fatal(err)
		}
		if reserveErr == nil {
			if token == "" || f.count(t) != 1 {
				t.Fatal("successful reservation lost durable lease")
			}
			if err := f.store.Release(context.Background(), f.request.Caller, f.request.Sandbox, token); err != nil {
				t.Fatal(err)
			}
		} else if f.count(t) != 0 {
			t.Fatal("failed reservation created lease")
		}
		if _, err := f.store.Reserve(context.Background(), f.request); err == nil {
			t.Fatal("post-revocation Workspace lease was accepted")
		}
		if f.count(t) != 0 {
			t.Fatal("unexpected post-revocation durable lease")
		}
	}
}
