package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
)

func TestDaytonaExperimentalDisabledByDefaultRetainsInventoryButBlocksExecution(t *testing.T) {
	t.Setenv(DaytonaExperimentalEnvironment, "")
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "remote-quarantine.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	s := runtime.Sandboxes
	if s.DaytonaConfigured() {
		t.Fatal("Daytona remote backend should be disabled by default")
	}
	if !strings.Contains(s.DaytonaStatus(), "MENDER_DAYTONA_EXPERIMENTAL=enabled") {
		t.Fatalf("missing opt-in guidance: %s", s.DaytonaStatus())
	}
	owner := runtime.OwnerPrincipal()
	caller, err := governancedomain.NewContext(governancedomain.OriginInternal, owner, governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := credentialdomain.ParseReference("credential://daytona/example")
	if err != nil {
		t.Fatal(err)
	}
	template := sandboxdaytona.Template{CredentialRef: ref, ImageDigest: "docker.io/library/python@sha256:" + strings.Repeat("a", 64), Target: sandboxdaytona.TargetUS, CPU: 1, MemoryGiB: 2, DiskGiB: 3, TTLMinutes: 30}
	if _, err := s.CreateDaytonaSandbox(ctx, CreateDaytonaSandboxRequest{Name: "No charge", Template: template}, caller); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("remote create available by default: %v", err)
	}
	id := sandboxdomain.ID("sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Persisted remote before quarantine", Backend: sandboxdomain.BackendDaytona, Isolation: sandboxdomain.IsolationRemote, OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.daytonaTemplates.CreateDefinedSandbox(ctx, item, template); err != nil {
		t.Fatal(err)
	}
	original, err := s.store.GetRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	provisional, err := s.store.TransitionRun(ctx, original, sandboxdomain.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	ready, err := s.store.TransitionRun(ctx, provisional, sandboxdomain.StateReady, "remote-id-persisted")
	if err != nil {
		t.Fatal(err)
	}
	if ready.State != sandboxdomain.StateReady {
		t.Fatal("test fixture failed to create persistent Ready remote Run")
	}
	visible, err := s.Get(ctx, id)
	if err != nil || visible.Backend() != sandboxdomain.BackendDaytona {
		t.Fatalf("remote inventory disappeared: %+v %v", visible, err)
	}
	if s.BackendAvailable(visible) {
		t.Fatal("unverified Daytona reported ready for new executions")
	}
	for _, capability := range []sandboxdomain.Capability{sandboxdomain.CapabilityRead, sandboxdomain.CapabilitySearch, sandboxdomain.CapabilityEdit, sandboxdomain.CapabilityShell} {
		placement, err := s.router.Resolve(ctx, id, capability)
		if err != nil {
			t.Fatalf("can't resolve safe metadata: %v", err)
		}
		lease, err := s.router.Acquire(ctx, caller, placement)
		if lease != nil || !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
			t.Fatalf("remote capability %s acquired without opt-in: %v", capability, err)
		}
	}
	active, err := s.ActiveLeases(ctx, id)
	if err != nil || active != 0 {
		t.Fatalf("denied acquisition created a lease: %d %v", active, err)
	}
	for _, action := range []governancedomain.Action{governancedomain.ActionSandboxStart, governancedomain.ActionSandboxStop, governancedomain.ActionSandboxDestroy} {
		if _, err := s.ChangeLifecycle(ctx, id, action, caller); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
			t.Fatalf("remote lifecycle %s reached SDK without opt-in: %v", action, err)
		}
	}
	if _, err := s.ReconcileDaytona(ctx, id, caller); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("remote reconcile reached SDK without opt-in: %v", err)
	}
	refs, err := s.DaytonaCredentialBindings(ctx, ref)
	if err != nil || len(refs) != 1 || refs[0] != id {
		t.Fatalf("disabled remote lost Credential Reference protection: %+v %v", refs, err)
	}
}

func TestDaytonaExperimentalRequiresExactHostOptIn(t *testing.T) {
	t.Setenv(DaytonaExperimentalEnvironment, "true")
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "remote-bad-flag.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	if runtime.Sandboxes.DaytonaConfigured() {
		t.Fatal("unspecific experimental flag was accepted")
	}
}
func TestDaytonaExperimentalExplicitOptInDoesNotContactRemoteAtStartup(t *testing.T) {
	t.Setenv(DaytonaExperimentalEnvironment, "enabled")
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "remote-opted-in.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	if !runtime.Sandboxes.DaytonaConfigured() {
		t.Fatal("explicit operator opt-in did not register remote backend")
	}
	if !strings.Contains(runtime.Sandboxes.DaytonaStatus(), "Experimental") {
		t.Fatalf("misleading status: %s", runtime.Sandboxes.DaytonaStatus())
	}
}
