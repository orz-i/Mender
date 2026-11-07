package typedlease_test

import (
	"context"
	"errors"
	"testing"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxrouter "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/router"
	"github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/typedlease"
)

type nativeRouteFixture struct {
	item sandbox.Sandbox
	run  sandbox.Run
}

func (f nativeRouteFixture) Get(context.Context, sandbox.ID) (sandbox.Sandbox, error) {
	return f.item, nil
}
func (f nativeRouteFixture) GetRun(context.Context, sandbox.ID) (sandbox.Run, error) {
	return f.run, nil
}
func (nativeRouteFixture) AdmitAcquire(context.Context, governance.Context, sandboxapp.Placement) error {
	return nil
}

type nativeRouteSession struct{ closed bool }

func (s *nativeRouteSession) Close() error { s.closed = true; return nil }

type nativeRouteBackend struct {
	opens   int
	session nativeRouteSession
}

func (*nativeRouteBackend) Kind() sandbox.BackendKind { return sandbox.BackendLocalWorkspace }
func (*nativeRouteBackend) Capabilities() sandbox.Capabilities {
	return sandbox.Capabilities{Read: true}
}
func (b *nativeRouteBackend) Open(context.Context, sandbox.Sandbox, sandbox.Run) (sandboxapp.Session, error) {
	b.opens++
	return &b.session, nil
}

type nativeRouteResolver struct{ backend *nativeRouteBackend }

func (r nativeRouteResolver) ResolveBackend(kind sandbox.BackendKind, binding sandbox.BackendBinding, cap sandbox.Capability) (sandboxapp.Backend, error) {
	if kind != sandbox.BackendLocalWorkspace || binding != sandbox.BuiltinBinding(kind) || cap != sandbox.CapabilityRead {
		return nil, sandbox.ErrCapabilityUnavailable
	}
	return r.backend, nil
}
func (r nativeRouteResolver) Available(kind sandbox.BackendKind, binding sandbox.BackendBinding) bool {
	return kind == sandbox.BackendLocalWorkspace && binding == sandbox.BuiltinBinding(kind)
}

func TestNativeSandboxRouterUsesAtomicLeaseWithoutSecondExecutionEngine(t *testing.T) {
	f := atomicWorkspaceFixture(t)
	ctx := context.Background()
	var root, ownerKind, ownerRef string
	err := f.db.QueryRowContext(ctx, "SELECT local_root,owner_kind,owner_ref FROM sandboxes WHERE id=?", f.request.Sandbox.String()).Scan(&root, &ownerKind, &ownerRef)
	if err != nil {
		t.Fatal(err)
	}
	item, err := sandbox.NewSandbox(sandbox.Spec{
		ID: f.request.Sandbox, Name: "Native", Backend: sandbox.BackendLocalWorkspace,
		Binding:   sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace),
		Isolation: f.request.Isolation, LocalRoot: root, OwnerKind: ownerKind, OwnerRef: ownerRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandbox.NewRun(item.ID(), f.request.Generation, sandbox.StateReady, "local-ready")
	if err != nil {
		t.Fatal(err)
	}
	fixture := nativeRouteFixture{item: item, run: run}
	backend := &nativeRouteBackend{}
	router, err := sandboxrouter.New(fixture, fixture, fixture, f.store, nativeRouteResolver{backend})
	if err != nil {
		t.Fatal(err)
	}
	placement, err := router.Resolve(ctx, item.ID(), sandbox.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	// A matching ToolID or SandboxId without the approved native request
	// cannot consume the same Router's lease path.
	if _, err := router.AcquireNative(ctx, f.request.Caller, placement); !errors.Is(err, typedlease.ErrUnadmitted) {
		t.Fatalf("unbound Native request acquired Sandbox: %v", err)
	}
	if backend.opens != 0 || f.count(t) != 0 {
		t.Fatal("unapproved call opened backend")
	}
	bound, err := typedlease.WithRequest(ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := router.AcquireNative(bound, f.request.Caller, placement)
	if err != nil {
		t.Fatal(err)
	}
	if backend.opens != 1 || f.count(t) != 1 {
		t.Fatal("native bound execution skipped durable lease")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if f.count(t) != 0 || !backend.session.closed {
		t.Fatal("native lease/backend cleanup leaked")
	}
	f.exec(t, "DELETE FROM workspace_profile_grants WHERE workspace_id=? AND profile_id=?", f.request.Workspace.String(), profileID)
	if _, err := router.AcquireNative(bound, f.request.Caller, placement); err == nil {
		t.Fatal("revoked Workspace grant was bypassed through Sandbox Router")
	}
	if backend.opens != 1 || f.count(t) != 0 {
		t.Fatal("revoked grant opened backend or leaked lease")
	}
}
