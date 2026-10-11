package router

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type fakeStore struct {
	item       sandboxdomain.Sandbox
	run        sandboxdomain.Run
	leaseMu    sync.Mutex
	leaseToken string
}

func (f *fakeStore) Get(context.Context, sandboxdomain.ID) (sandboxdomain.Sandbox, error) {
	return f.item, nil
}
func (f *fakeStore) GetRun(context.Context, sandboxdomain.ID) (sandboxdomain.Run, error) {
	return f.run, nil
}

func (f *fakeStore) ReserveExecution(_ context.Context, id sandboxdomain.ID, generation uint64, caller gov.Context, capability sandboxdomain.Capability) (string, error) {
	if err := capability.Validate(); err != nil {
		return "", err
	}
	f.leaseMu.Lock()
	defer f.leaseMu.Unlock()
	if id != f.item.ID() || f.run.State != sandboxdomain.StateReady || generation != f.run.Generation || f.leaseToken != "" {
		return "", sandboxdomain.ErrConflict
	}
	if err := caller.Validate(); err != nil {
		return "", err
	}
	f.leaseToken = "0123456789abcdef0123456789abcdef"
	return f.leaseToken, nil
}
func (f *fakeStore) ReleaseExecution(_ context.Context, id sandboxdomain.ID, token string) error {
	f.leaseMu.Lock()
	defer f.leaseMu.Unlock()
	if id != f.item.ID() || f.leaseToken == "" || token != f.leaseToken {
		return fmt.Errorf("release wrong Sandbox lease: %w", sandboxdomain.ErrConflict)
	}
	f.leaseToken = ""
	return nil
}

type gate struct {
	denied bool
	calls  atomic.Int32
}

func (g *gate) AdmitAcquire(_ context.Context, _ gov.Context, _ sandboxapp.Placement) error {
	g.calls.Add(1)
	if g.denied {
		return sandboxdomain.ErrNotAuthorized
	}
	return nil
}

type session struct{ closed atomic.Int32 }

func (s *session) Close() error { s.closed.Add(1); return nil }

type backend struct {
	opens   atomic.Int32
	session *session
	cap     sandboxdomain.Capabilities
}

func (*backend) Kind() sandboxdomain.BackendKind            { return sandboxdomain.BackendLocalWorkspace }
func (b *backend) Capabilities() sandboxdomain.Capabilities { return b.cap }
func (b *backend) Open(_ context.Context, _ sandboxdomain.Sandbox, _ sandboxdomain.Run) (sandboxapp.Session, error) {
	b.opens.Add(1)
	return b.session, nil
}
func setup(t *testing.T) (*fakeStore, gov.Context, *gate, *backend, *Router) {
	t.Helper()
	principal, err := gov.NewPrincipal(gov.PrincipalLocalUser, "owner")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginCLI, principal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	id := sandboxdomain.ID("sbx_0123456789abcdef0123456789abcdef")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Test", Backend: sandboxdomain.BackendLocalWorkspace, Isolation: sandboxdomain.IsolationHost, LocalRoot: t.TempDir(), OwnerKind: "local_user", OwnerRef: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "local-ready")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeStore{item: item, run: run}
	g := &gate{}
	b := &backend{session: &session{}, cap: sandboxdomain.Capabilities{Read: true}}
	r, err := New(f, f, g, f, b)
	if err != nil {
		t.Fatal(err)
	}
	return f, caller, g, b, r
}
func TestResolveHasNoBackendEffectsAndDeniedAcquire(t *testing.T) {
	_, caller, g, b, r := setup(t)
	placement, err := r.Resolve(context.Background(), sandboxdomain.ID("sbx_0123456789abcdef0123456789abcdef"), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if b.opens.Load() != 0 || g.calls.Load() != 0 {
		t.Fatal("resolve caused side effects")
	}
	g.denied = true
	if _, err := r.Acquire(context.Background(), caller, placement); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("deny: %v", err)
	}
	if b.opens.Load() != 0 {
		t.Fatal("denied acquire opened backend")
	}
}
func TestAcquireCapabilityLifecycleAndNoFallback(t *testing.T) {
	f, caller, g, b, r := setup(t)
	ctx := context.Background()
	_ = g
	place, err := r.Resolve(ctx, f.item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := r.Acquire(ctx, caller, place)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Generation != 1 || b.opens.Load() != 1 {
		t.Fatalf("lease: %+v", lease)
	}
	if _, err := r.Acquire(ctx, caller, place); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("expected capacity rejection: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if b.session.closed.Load() != 1 {
		t.Fatal("lease not idempotent")
	}
	other, err := r.Resolve(ctx, f.item.ID(), sandboxdomain.CapabilityShell)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Acquire(ctx, caller, other); !errors.Is(err, sandboxdomain.ErrCapabilityUnavailable) {
		t.Fatalf("capability fallback: %v", err)
	}
	if b.opens.Load() != 1 {
		t.Fatal("unexpected fallback execution")
	}
	f.run.Generation++
	if _, err := r.Acquire(ctx, caller, place); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stale generation: %v", err)
	}
}
func TestFailClosedWithoutAdmissionGateOrBackend(t *testing.T) {
	f, caller, _, _, _ := setup(t)
	if _, err := New(f, f, nil, f); err == nil {
		t.Fatal("missing gate accepted")
	}
	g := &gate{}
	r, err := New(f, f, g, f)
	if err != nil {
		t.Fatal(err)
	}
	place, err := r.Resolve(context.Background(), f.item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Acquire(context.Background(), caller, place); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("missing backend: %v", err)
	}
}
