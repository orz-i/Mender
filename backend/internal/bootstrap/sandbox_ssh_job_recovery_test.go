package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

func TestSSHJobRecoveryOwnerInspectionAndNoImplicitResolution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	template, secret, _ := startInProcessSSHForSandbox(t)
	template.User = "root"
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "recovery.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := gov.NewContext(gov.OriginInternal, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := runtime.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "Recovery inspector fixture", Template: template}, owner)
	if err != nil {
		t.Fatal(err)
	}
	otherPrincipal, err := gov.NewPrincipal(gov.PrincipalLocalUser, "os:someone-else")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := gov.NewContext(gov.OriginCLI, otherPrincipal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Sandboxes.InspectSSHJobRecovery(ctx, item.ID(), outsider); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("other principal could inspect SSH jobs: %v", err)
	}
	initial, err := runtime.Sandboxes.InspectSSHJobRecovery(ctx, item.ID(), owner)
	if err != nil || initial.Managed || initial.Evidence.Verdict != "not_enrolled" || len(initial.Evidence.Records) != 0 {
		t.Fatalf("fresh SSH issued plugin side effects: %+v %v", initial, err)
	}
	// The in-process SSH fixture has SFTP but cannot start systemd, so the
	// job manifests are reliably left UNKNOWN with no real remote command.
	submitted, err := runtime.Sandboxes.SubmitSSHJob(ctx, item.ID(), owner, sshremote.JobRequest{IdempotencyKey: strings.Repeat("9", 32), Command: "echo never-run"})
	if !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("expected unknown fixture: %+v %v", submitted, err)
	}
	inspected, err := runtime.Sandboxes.InspectSSHJobRecovery(ctx, item.ID(), owner)
	if err != nil || !inspected.Managed || inspected.Evidence.Verdict != "uncertain" || len(inspected.Evidence.Records) != 1 {
		t.Fatalf("owner recovery evidence unavailable: %+v %v", inspected, err)
	}
	record := inspected.Evidence.Records[0]
	if record.TaskID != submitted.TaskID || record.Activity != "uncertain" || record.Recoverable || record.Fingerprint == "" {
		t.Fatalf("inspector fabricated v1 recovery: %+v", record)
	}
	count, err := runtime.Sandboxes.ActiveLeases(ctx, item.ID())
	if err != nil || count != 0 {
		t.Fatalf("observation acquired exclusive lease: %d %v", count, err)
	}
	if _, err = runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, owner); !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("inspection cleared uncertain lifecycle: %v", err)
	}
}
