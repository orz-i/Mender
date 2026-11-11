package sandboxbridge

import (
	"context"
	"errors"
	"testing"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type routerFixture struct {
	placement              sandboxapp.Placement
	lease                  *sandboxapp.Lease
	resolveErr, acquireErr error
	resolves, acquires     int
	id                     sandbox.ID
	capability             sandbox.Capability
}

func (r *routerFixture) Resolve(_ context.Context, id sandbox.ID, c sandbox.Capability) (sandboxapp.Placement, error) {
	r.resolves++
	r.id = id
	r.capability = c
	return r.placement, r.resolveErr
}
func (r *routerFixture) Acquire(context.Context, governance.Context, sandboxapp.Placement) (*sandboxapp.Lease, error) {
	r.acquires++
	return r.lease, r.acquireErr
}

type closeOnly struct{}

func (closeOnly) Close() error { return nil }

type typedFixture struct {
	closeOnly
	calls int
	read  workspace.ReadRequest
}

func (s *typedFixture) Read(_ context.Context, r workspace.ReadRequest) (workspace.ReadResult, error) {
	s.calls++
	s.read = r
	return workspace.ReadResult{Path: r.Path}, nil
}
func (*typedFixture) Search(context.Context, workspace.SearchRequest) (workspace.SearchResult, error) {
	return workspace.SearchResult{}, nil
}
func (*typedFixture) Edit(context.Context, workspace.EditRequest) (workspace.EditResult, error) {
	return workspace.EditResult{}, nil
}
func (*typedFixture) Shell(context.Context, workspace.ShellRequest) (workspace.ShellResult, error) {
	return workspace.ShellResult{}, nil
}

func bridgeFixture(t *testing.T) (*routerFixture, *typedFixture, *int) {
	t.Helper()
	item, err := sandbox.NewSandbox(sandbox.Spec{ID: "sbx_0123456789abcdef0123456789abcdef", Name: "typed bridge", Backend: sandbox.BackendLocalWorkspace, Binding: sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace), Isolation: sandbox.IsolationHost, LocalRoot: t.TempDir(), OwnerKind: "local_user", OwnerRef: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	s := &typedFixture{}
	closed := new(int)
	r := &routerFixture{placement: sandboxapp.Placement{Sandbox: item, Required: sandbox.CapabilityRead}}
	r.lease = sandboxapp.NewLease(item.ID(), 1, s, func() error { *closed++; return nil })
	return r, s, closed
}

func TestTypedBridgePinsTargetCapabilityAndOriginalCleanup(t *testing.T) {
	r, s, closed := bridgeFixture(t)
	target := r.placement.Sandbox.ID().String()
	l, err := (Provider{Router: r}).Acquire(context.Background(), governance.Context{}, target, workspace.Read)
	if err != nil {
		t.Fatal(err)
	}
	out, err := l.Read(context.Background(), workspace.ReadRequest{Path: "a"})
	if err != nil || out.Path != "a" || s.read.Path != "a" || *closed != 1 || r.id.String() != target || r.capability != sandbox.CapabilityRead {
		t.Fatalf("routing or lifetime: %+v %v %+v %d", out, err, r, *closed)
	}
}

func TestBridgeRejectsUnknownOrConflictingPlacementBeforeAcquire(t *testing.T) {
	for _, operation := range []workspace.Operation{"unknown", workspace.Read, workspace.Shell} {
		r, _, _ := bridgeFixture(t)
		target := r.placement.Sandbox.ID().String()
		if operation == workspace.Read {
			target = "sbx_abcdefabcdefabcdefabcdefabcdefab"
		}
		_, err := (Provider{Router: r}).Acquire(context.Background(), governance.Context{}, target, operation)
		if err == nil || r.acquires != 0 {
			t.Fatalf("unsafe acquire %s: %v count=%d", operation, err, r.acquires)
		}
		if operation == "unknown" && (!errors.Is(err, sandbox.ErrCapabilityUnavailable) || r.resolves != 0) {
			t.Fatalf("unknown operation reached router: %v", err)
		}
	}
}

func TestBridgeUnsupportedSessionAndErrorsDoNotLeakLease(t *testing.T) {
	r, _, closed := bridgeFixture(t)
	closeErr := errors.New("close failed")
	r.lease = sandboxapp.NewLease(r.placement.Sandbox.ID(), 1, closeOnly{}, func() error { *closed++; return closeErr })
	_, err := (Provider{Router: r}).Acquire(context.Background(), governance.Context{}, r.placement.Sandbox.ID().String(), workspace.Read)
	if !errors.Is(err, sandbox.ErrCapabilityUnavailable) || !errors.Is(err, closeErr) || *closed != 1 {
		t.Fatalf("unsupported session cleanup: %v %d", err, *closed)
	}
	r, _, _ = bridgeFixture(t)
	r.resolveErr = context.Canceled
	_, err = (Provider{Router: r}).Acquire(context.Background(), governance.Context{}, r.placement.Sandbox.ID().String(), workspace.Read)
	if !errors.Is(err, context.Canceled) || r.acquires != 0 {
		t.Fatalf("resolve cancellation lost: %v", err)
	}
	r, _, _ = bridgeFixture(t)
	r.lease = nil
	if _, err = (Provider{Router: r}).Acquire(context.Background(), governance.Context{}, r.placement.Sandbox.ID().String(), workspace.Read); err == nil {
		t.Fatal("missing lease accepted")
	}
}
