package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestRuntimeSandboxLocalBackendUsesGovernedLeasesAndPersists(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "mender.db")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("local sandbox"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	id := sandbox.ID("sbx_0123456789abcdef0123456789abcdef")
	owner := runtime.OwnerPrincipal()
	item, err := sandbox.NewSandbox(sandbox.Spec{ID: id, Name: "Trusted Local", Backend: sandbox.BackendLocalWorkspace, Binding: sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace), Isolation: sandbox.IsolationHost, LocalRoot: root, OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Sandboxes.store.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	run, err := sandbox.NewRun(id, 1, sandbox.StateReady, "local-workspace-ready")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Sandboxes.store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	list, err := runtime.Sandboxes.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("sandbox list %+v %v", list, err)
	}
	placement, err := runtime.Sandboxes.router.Resolve(ctx, id, sandbox.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	principalCtx, err := governance.NewContext(governance.OriginInternal, owner, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := runtime.Sandboxes.router.Acquire(ctx, principalCtx, placement)
	if err != nil {
		t.Fatalf("registered local backend unavailable: %v", err)
	}
	workspace, ok := lease.Session.(sandboxapplication.WorkspaceSession)
	if !ok {
		t.Fatalf("backend session does not expose Workspace operations: %T", lease.Session)
	}
	args, err := invocationdomain.NewJSONValue([]byte(`{"path":"notes.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	read, err := workspace.ExecuteWorkspace(ctx, sandbox.CapabilityRead, args)
	if err != nil || read.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("sandbox read: %v %s", err, read.Kind())
	}
	var result struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(read.Content().Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Content != "local sandbox" {
		t.Fatalf("read %q", result.Content)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	// A remote backend is never silently substituted with Local Workspace.
	remoteID := sandbox.ID("sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	remote, err := sandbox.NewSandbox(sandbox.Spec{ID: remoteID, Name: "Unavailable Remote", Backend: sandbox.BackendDaytona, Binding: sandbox.BuiltinBinding(sandbox.BackendDaytona), Isolation: sandbox.IsolationRemote, OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Sandboxes.store.Create(ctx, remote); err != nil {
		t.Fatal(err)
	}
	remoteRun, err := sandbox.NewRun(remoteID, 1, sandbox.StateReady, "remote-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Sandboxes.store.CreateRun(ctx, remoteRun); err != nil {
		t.Fatal(err)
	}
	remotePlacement, err := runtime.Sandboxes.router.Resolve(ctx, remoteID, sandbox.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Sandboxes.router.Acquire(ctx, principalCtx, remotePlacement); !errors.Is(err, sandbox.ErrBackendUnavailable) {
		t.Fatalf("remote silently fell back: %v", err)
	}
	admissions, err := runtime.Sandboxes.store.ListSandboxAdmissions(ctx, id, 10)
	if err != nil || len(admissions) != 1 || admissions[0].Status != "admitted" {
		t.Fatalf("admission audit %+v %v", admissions, err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	loaded, err := reopened.Sandboxes.Get(ctx, id)
	if err != nil || loaded.Spec() != item.Spec() {
		t.Fatalf("reopened %+v %v", loaded, err)
	}
	observed, err := reopened.Sandboxes.store.GetRun(ctx, id)
	if err != nil || observed != run {
		t.Fatalf("run %+v %v", observed, err)
	}
}

func TestSandboxProvisionDeniedBeforeFilesystemAccessAndAudited(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "provision.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: databasePath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	if _, err = runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{
		DisplayName: "deny Sandbox provision", Enabled: true, Decision: policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{Actions: []governance.Action{governance.ActionSandboxProvision}},
	}); err != nil {
		t.Fatal(err)
	}
	principal := runtime.OwnerPrincipal()
	caller, err := governance.NewContext(governance.OriginInternal, principal, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	// A missing root would fail only if the denied operation probed the filesystem.
	_, err = runtime.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "Denied", Root: filepath.Join(t.TempDir(), "not-created"), Isolation: sandbox.IsolationHost,
	}, caller)
	if !errors.Is(err, sandbox.ErrNotAuthorized) {
		t.Fatalf("denied action: %v", err)
	}
	items, err := runtime.Sandboxes.List(ctx)
	if err != nil || len(items) != 0 {
		t.Fatalf("denied creation persisted Sandbox: %+v %v", items, err)
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var denied int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_admissions WHERE action='sandbox.provision' AND status='denied'`).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if denied != 1 {
		t.Fatalf("provision denials audited: %d", denied)
	}
}

func TestLocalSandboxProvisionAndSandboxLeaseShareOwner(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "provision.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	principal := runtime.OwnerPrincipal()
	caller, err := governance.NewContext(governance.OriginInternal, principal, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("sandbox"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, err := runtime.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{Name: "Local sandbox", Root: root, Isolation: sandbox.IsolationHost}, caller)
	if err != nil {
		t.Fatal(err)
	}
	run, err := runtime.Sandboxes.store.GetRun(ctx, item.ID())
	if err != nil || run.State != sandbox.StateReady {
		t.Fatalf("run %+v, %v", run, err)
	}
	placement, err := runtime.Sandboxes.router.Resolve(ctx, item.ID(), sandbox.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := runtime.Sandboxes.router.Acquire(ctx, caller, placement)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Close() }()
	ws, ok := lease.Session.(sandboxapplication.WorkspaceSession)
	if !ok {
		t.Fatalf("not WorkspaceSession %T", lease.Session)
	}
	args, err := invocationdomain.NewJSONValue([]byte(`{"path":"hello.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := ws.ExecuteWorkspace(ctx, sandbox.CapabilityRead, args)
	if err != nil || outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("read %+v %v", outcome, err)
	}
	admitted, err := runtime.Sandboxes.store.ListSandboxAdmissions(ctx, item.ID(), 10)
	if err != nil || len(admitted) != 2 || admitted[0].Action != governance.ActionSandboxAcquire || admitted[1].Action != governance.ActionSandboxProvision {
		t.Fatalf("admissions %+v %v", admitted, err)
	}
}
