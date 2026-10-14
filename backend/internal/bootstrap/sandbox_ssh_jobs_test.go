package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

func TestSSHManagedJobsV2UnknownOutcomeBlocksLifecycleAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	tpl, secret, root := startInProcessSSHForSandbox(t)
	tpl.User = "root"
	database := filepath.Join(t.TempDir(), "managed-v2-jobs.sqlite")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := gov.NewContext(gov.OriginInternal, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := runtime.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "managed v2 jobs SSH", Template: tpl}, owner)
	if err != nil {
		t.Fatal(err)
	}
	managed, err := runtime.Sandboxes.store.SSHJobManaged(ctx, item.ID())
	if err != nil || managed {
		t.Fatal("Jobs implicitly enrolled")
	}
	// Unsupported fsync/exec must fail BEFORE creating a TaskID or obligation.
	_, err = runtime.Sandboxes.SubmitSSHJob(ctx, item.ID(), owner, sshremote.JobRequest{IdempotencyKey: strings.Repeat("a", 32), Command: "echo never"})
	if !errors.Is(err, sshremote.ErrJobUnavailable) {
		t.Fatalf("unsupported v2 host accepted submission: %v", err)
	}
	managed, err = runtime.Sandboxes.store.SSHJobManaged(ctx, item.ID())
	if err != nil || managed {
		t.Fatal("unsupported v2 created Core obligation")
	}
	// Enroll and stage plugin-owned test fixtures without needing real systemd.
	enrollSSHJobV2Fixture(t, ctx, runtime, item.ID(), owner)
	job := seedSSHJobV2(t, root, item.ID(), "sjob_"+strings.Repeat("a", 32), false)
	if _, err = runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStop, owner); !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("v2 unknown allowed Stop: %v", err)
	}
	if _, err = runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, owner); !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("v2 unknown allowed Destroy: %v", err)
	}
	if err = runtime.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	reopened.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	nextOwner, err := gov.NewContext(gov.OriginInternal, reopened.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStop, nextOwner); !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("restart forgot unknown v2: %v", err)
	}
	if err = os.WriteFile(filepath.Join(job.Dir, "start.intent"), []byte(job.Start), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(job.Dir, "exit"), []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stopped, err := reopened.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStop, nextOwner)
	if err != nil || stopped.State != sandboxdomain.StateStopped {
		t.Fatalf("proved v2 terminal Stop: %+v %v", stopped, err)
	}
	if _, err = reopened.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, nextOwner); err != nil {
		t.Fatalf("v2 terminal Destroy: %v", err)
	}
}
