package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

func TestSSHManagedJobsUnknownOutcomeBlocksLifecycleAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	tpl, secret, root := startInProcessSSHForSandbox(t)
	// Test fixture accepts the selected account's pinned key; no real root
	// privileges or systemd service is requested from the local SSH server.
	tpl.User = "root"
	database := filepath.Join(t.TempDir(), "managed-jobs.sqlite")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := gov.NewContext(gov.OriginInternal, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := runtime.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "managed jobs SSH", Template: tpl}, owner)
	if err != nil {
		t.Fatal(err)
	}
	// Before a job is enrolled, no remote task inventory is needed by existing
	// passive SSH lifecycle; preexisting behavior is covered by SSH integration.
	enrolled, err := runtime.Sandboxes.store.SSHJobManaged(ctx, item.ID())
	if err != nil || enrolled {
		t.Fatal("Jobs were enabled implicitly")
	}
	// In-process test SSH intentionally has only SFTP, no systemd or exec RPC.
	// Durable reservation succeeds but missing systemd ACK remains UNKNOWN.
	key := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	job, err := runtime.Sandboxes.SubmitSSHJob(ctx, item.ID(), owner, sshremote.JobRequest{IdempotencyKey: key, Command: "echo side-effect-once"})
	if !errors.Is(err, sshremote.ErrJobUnknown) || job.State != sshremote.JobUnknown || job.TaskID == "" {
		t.Fatalf("unknown remote submit %+v %v", job, err)
	}
	leases, err := runtime.Sandboxes.ActiveLeases(ctx, item.ID())
	if err != nil || leases != 0 {
		t.Fatalf("detached job leaked short lease: %d %v", leases, err)
	}
	enrolled, err = runtime.Sandboxes.store.SSHJobManaged(ctx, item.ID())
	if err != nil || !enrolled {
		t.Fatal("plugin task admission did not retain obligation")
	}
	if _, err = runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStop, owner); !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("uncertain job allowed stop: %v", err)
	}
	if _, err = runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, owner); !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("uncertain job allowed destroy: %v", err)
	}
	if run, err := runtime.Sandboxes.GetRun(ctx, item.ID()); err != nil || run.State != sandboxdomain.StateReady {
		t.Fatalf("uncertain lifecycle changed state %+v %v", run, err)
	}
	// A second Mender process must recover the same obligation from SQLite and
	// the same remote task identity from the plugin's on-host private store.
	if err = runtime.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	restarted.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	recoveredOwner, err := gov.NewContext(gov.OriginInternal, restarted.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStop, recoveredOwner); !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("restart forgot unknown job: %v", err)
	}
	// Test-owned SSH server does not run the command. We simulate only the
	// durable terminal artifact that a real on-host wrapper would have written.
	resultPath := filepath.Join(root+".mender-ssh-jobs-v1", item.ID().String(), job.TaskID, "exit")
	// #nosec G306 -- only the test-owned temporary remote store receives a terminal marker.
	if err = os.WriteFile(resultPath, []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stopped, err := restarted.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStop, recoveredOwner)
	if err != nil || stopped.State != sandboxdomain.StateStopped {
		t.Fatalf("proved-terminal job didn't permit stop: %+v %v", stopped, err)
	}
	if leases, err = restarted.Sandboxes.ActiveLeases(ctx, item.ID()); err != nil || leases != 0 {
		t.Fatalf("lifecycle fence leaked: %d %v", leases, err)
	}
	if _, err = restarted.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, recoveredOwner); err != nil {
		t.Fatalf("proved-terminal job didn't permit cleanup: %v", err)
	}
}
