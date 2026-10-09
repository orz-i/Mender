package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
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
	item, err := sandbox.NewSandbox(sandbox.Spec{ID: id, Name: "Trusted Local", Backend: sandbox.BackendLocalWorkspace, Isolation: sandbox.IsolationHost, LocalRoot: root, OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()})
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
	remote, err := sandbox.NewSandbox(sandbox.Spec{ID: remoteID, Name: "Unavailable Remote", Backend: sandbox.BackendDaytona, Isolation: sandbox.IsolationRemote, OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()})
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
