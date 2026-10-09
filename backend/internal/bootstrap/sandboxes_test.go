package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestRuntimeSandboxInventoryPersistsAndRouterFailsClosed(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "mender.db")
	root := t.TempDir()
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
	if _, err := runtime.Sandboxes.router.Acquire(ctx, principalCtx, placement); !errors.Is(err, sandbox.ErrBackendUnavailable) {
		t.Fatalf("unregistered backend allowed: %v", err)
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
