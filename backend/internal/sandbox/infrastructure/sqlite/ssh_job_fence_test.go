package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func newFencedSSHFixture(t *testing.T) (string, sandboxdomain.ID, sandboxdomain.Run, gov.Context) {
	t.Helper()
	ctx := context.Background()
	filename := filepath.Join(t.TempDir(), "job-fence.sqlite")
	store, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := gov.NewPrincipal(gov.PrincipalLocalUser, "os:fixture")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginCLI, principal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	id := sandboxdomain.ID("sbx_000000000000000000000000000000ad")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "SSH job fence", Backend: sandboxdomain.BackendSSHRemote,
		Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote), Isolation: sandboxdomain.IsolationRemote,
		OwnerKind: "local_user", OwnerRef: principal.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "ssh_remote:"+id.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateReady(ctx, item, run); err != nil {
		t.Fatal(err)
	}
	// The isolated store has no Provider catalogue. The existing lifecycle
	// guard queries this shape only for a Sandbox Destroy.
	if _, err := store.db.ExecContext(ctx, `CREATE TABLE provider_configs(provider_id TEXT,plugin_id TEXT,plugin_installation_id TEXT,config_version INTEGER,config_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return filename, id, run, caller
}
func TestSSHJobLifecycleFenceAtomicAcrossRuntimes(t *testing.T) {
	filename, id, run, caller := newFencedSSHFixture(t)
	ctx := context.Background()
	first, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	managed, err := first.SSHJobManaged(ctx, id)
	if err != nil || managed {
		t.Fatalf("fresh SSH incorrectly job-enrolled: %t %v", managed, err)
	}
	submit, err := first.ReserveSSHJobSubmit(ctx, id, run.Generation, caller)
	if err != nil {
		t.Fatal(err)
	}
	managed, err = second.SSHJobManaged(ctx, id)
	if err != nil || !managed {
		t.Fatalf("admitted job lost durable obligation: %t %v", managed, err)
	}
	// Job obligations cannot be silently erased or rewritten by an accidental
	// cleanup path, even if another Runtime bypasses the management use case.
	if _, err = second.db.ExecContext(ctx, `DELETE FROM sandbox_ssh_job_obligations WHERE sandbox_id=?`, id.String()); err == nil {
		t.Fatal("SSH Job obligation unexpectedly removable")
	}
	if _, err = second.db.ExecContext(ctx, `UPDATE sandbox_ssh_job_obligations SET enrolled_at='modified' WHERE sandbox_id=?`, id.String()); err == nil {
		t.Fatal("SSH Job obligation unexpectedly mutable")
	}
	if _, err = second.ReserveSSHJobLifecycle(ctx, run, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("Stop escaped submitting lease: %v", err)
	}
	if _, err = second.ChangeSSHRemoteLifecycle(ctx, run, gov.ActionSandboxStop, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unfenced stop escaped: %v", err)
	}
	if err = first.ReleaseExecution(ctx, id, submit); err != nil {
		t.Fatal(err)
	}
	fence, err := second.ReserveSSHJobLifecycle(ctx, run, caller)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.ReserveSSHJobSubmit(ctx, id, run.Generation, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("submit escaped lifecycle fence: %v", err)
	}
	if _, err = first.ChangeSSHRemoteLifecycleFenced(ctx, run, gov.ActionSandboxStop, caller, "0123456789abcdef0123456789abcdef"); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("wrong fence consumed lifecycle: %v", err)
	}
	unchanged, err := first.GetRun(ctx, id)
	if err != nil || unchanged.State != sandboxdomain.StateReady {
		t.Fatalf("failed fence changed run %+v %v", unchanged, err)
	}
	stopped, err := second.ChangeSSHRemoteLifecycleFenced(ctx, run, gov.ActionSandboxStop, caller, fence)
	if err != nil || stopped.State != sandboxdomain.StateStopped {
		t.Fatalf("atomic fenced stop %+v %v", stopped, err)
	}
	leases, err := first.ActiveLeaseCount(ctx, id)
	if err != nil || leases != 0 {
		t.Fatalf("fence not consumed: %d %v", leases, err)
	}
	if _, err = first.ReserveSSHJobSubmit(ctx, id, run.Generation, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stopped sandbox accepted job: %v", err)
	}
	restarted, err := first.ChangeSSHRemoteLifecycle(ctx, stopped, gov.ActionSandboxStart, caller)
	if err != nil || restarted.State != sandboxdomain.StateReady {
		t.Fatalf("start after job finish %+v %v", restarted, err)
	}
	if _, err = second.ChangeSSHRemoteLifecycle(ctx, restarted, gov.ActionSandboxDestroy, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unfenced destroy escaped: %v", err)
	}
	fence, err = second.ReserveSSHJobLifecycle(ctx, restarted, caller)
	if err != nil {
		t.Fatal(err)
	}
	destroyed, err := second.ChangeSSHRemoteLifecycleFenced(ctx, restarted, gov.ActionSandboxDestroy, caller, fence)
	if err != nil || destroyed.State != sandboxdomain.StateDestroyed {
		t.Fatalf("fenced destroy %+v %v", destroyed, err)
	}
	managed, err = second.SSHJobManaged(ctx, id)
	if err != nil || !managed {
		t.Fatal("task obligation lost after destroy")
	}
}

// Submit and Stop race on two independent SQLite connections. Exactly one
// can acquire the resource's execution lease / lifecycle transition. No
// timing-dependent sequence can produce both an accepted submission and a
// completed stop, including before the managed marker first exists.
func TestSSHJobFirstSubmitRacesPassiveStopAcrossRuntimes(t *testing.T) {
	for iteration := 0; iteration < 12; iteration++ {
		filename, id, run, caller := newFencedSSHFixture(t)
		ctx := context.Background()
		first, err := Open(ctx, filename)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Open(ctx, filename)
		if err != nil {
			_ = first.Close()
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		var submitToken string
		var submitErr, stopErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			submitToken, submitErr = first.ReserveSSHJobSubmit(ctx, id, run.Generation, caller)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, stopErr = second.ChangeSSHRemoteLifecycle(ctx, run, gov.ActionSandboxStop, caller)
		}()
		close(start)
		wg.Wait()
		if submitErr == nil && stopErr == nil {
			t.Fatalf("iteration %d admitted remote job AND stopped Sandbox", iteration)
		}
		managed, e := first.SSHJobManaged(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		current, e := first.GetRun(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		if submitErr == nil {
			if !managed || current.State != sandboxdomain.StateReady {
				t.Fatalf("iteration %d admitted task without protected Ready run: %+v managed=%t", iteration, current, managed)
			}
			if e = first.ReleaseExecution(ctx, id, submitToken); e != nil {
				t.Fatal(e)
			}
			if _, e = second.ChangeSSHRemoteLifecycle(ctx, run, gov.ActionSandboxStop, caller); !errors.Is(e, sandboxdomain.ErrConflict) {
				t.Fatalf("iteration %d unmanaged stop bypassed job obligation: %v", iteration, e)
			}
		} else if current.State == sandboxdomain.StateStopped && managed {
			t.Fatalf("iteration %d stopped with a permanent job obligation", iteration)
		}
		if e = second.Close(); e != nil {
			t.Fatal(e)
		}
		if e = first.Close(); e != nil {
			t.Fatal(e)
		}
	}
}

func TestSSHJobFenceAuthorizationAndNoEnrolledFalsePositive(t *testing.T) {
	filename, id, run, owner := newFencedSSHFixture(t)
	ctx := context.Background()
	store, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	otherPrincipal, _ := gov.NewPrincipal(gov.PrincipalLocalUser, "os:other")
	other, _ := gov.NewContext(gov.OriginCLI, otherPrincipal, gov.CatalogScope())
	if _, err = store.ReserveSSHJobSubmit(ctx, id, 1, other); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("other owner enrolled job: %v", err)
	}
	servicePrincipal, _ := gov.NewPrincipal(gov.PrincipalService, "profile:prof_00000000000000000000000000000011")
	service, _ := gov.NewContext(gov.OriginInternal, servicePrincipal, gov.CatalogScope())
	if _, err = store.ReserveSSHJobSubmit(ctx, id, 1, service); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("service bypassed owner-only seam: %v", err)
	}
	if _, err = store.ReserveSSHJobLifecycle(ctx, run, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unenrolled SSH required job fence: %v", err)
	}
	stopped, err := store.ChangeSSHRemoteLifecycle(ctx, run, gov.ActionSandboxStop, owner)
	if err != nil || stopped.State != sandboxdomain.StateStopped {
		t.Fatalf("existing passive lifecycle broken: %+v %v", stopped, err)
	}
	managed, err := store.SSHJobManaged(ctx, id)
	if err != nil || managed {
		t.Fatal("failed/unauthorized enrolment left marker")
	}
}
