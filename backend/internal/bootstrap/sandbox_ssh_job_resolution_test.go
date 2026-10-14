package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

func TestSSHRecoveryV2OwnerResolveIsFenced(t *testing.T) {
	ctx := context.Background()
	tpl, secret, root := startInProcessSSHForSandbox(t)
	tpl.User = "root"
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "job-r3b.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	runtime.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, _ := gov.NewContext(gov.OriginInternal, runtime.OwnerPrincipal(), gov.CatalogScope())
	item, err := runtime.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "v2 owner recovery fence", Template: tpl}, owner)
	if err != nil {
		t.Fatal(err)
	}
	enrollSSHJobV2Fixture(t, ctx, runtime, item.ID(), owner)
	submitted := seedSSHJobV2(t, root, item.ID(), "sjob_"+strings.Repeat("f", 32), false)
	observed, err := runtime.Sandboxes.InspectSSHJobRecovery(ctx, item.ID(), owner)
	if err != nil || len(observed.Evidence.Records) != 1 || !observed.Evidence.Records[0].Recoverable {
		t.Fatalf("cannot inspect fenced v2 task: %+v %v", observed, err)
	}
	fingerprint := observed.Evidence.Records[0].Fingerprint
	req := SSHJobResolutionRequest{TaskID: submitted.ID, Generation: observed.Generation,
		Fingerprint: fingerprint, Reason: "operator reviewed v2 prelaunch evidence"}
	if _, err = runtime.Sandboxes.ResolveSSHJobRecovery(ctx, item.ID(), req, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unpaused recovery was admitted: %v", err)
	}
	outsidePrincipal, _ := gov.NewPrincipal(gov.PrincipalLocalUser, "os:not-owner")
	outside, _ := gov.NewContext(gov.OriginCLI, outsidePrincipal, gov.CatalogScope())
	if _, err = runtime.Sandboxes.ResolveSSHJobRecovery(ctx, item.ID(), req, outside); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("another owner could recover Task: %v", err)
	}
	if _, err = runtime.Sandboxes.SetSSHJobSubmissionsPaused(ctx, item.ID(), true, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Sandboxes.ResolveSSHJobRecovery(ctx, item.ID(), SSHJobResolutionRequest{
		TaskID: submitted.ID, Generation: observed.Generation + 1, Fingerprint: fingerprint, Reason: req.Reason,
	}, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stale Run generation admitted: %v", err)
	}
	if _, err = runtime.Sandboxes.ResolveSSHJobRecovery(ctx, item.ID(), req, owner); err == nil {
		t.Fatal("unsupported v2 backend could not be resolved, but success was fabricated")
	}
	auditCount, err := runtime.Sandboxes.store.SSHRecoveryAuditCount(ctx, item.ID())
	if err != nil || auditCount != 1 {
		t.Fatalf("failed resolution not audited: %d %v", auditCount, err)
	}
	after, err := runtime.Sandboxes.InspectSSHJobRecovery(ctx, item.ID(), owner)
	if err != nil || len(after.Evidence.Records) != 1 ||
		after.Evidence.Records[0].Fingerprint != fingerprint ||
		after.Evidence.Records[0].Activity != "uncertain" ||
		!after.Evidence.Records[0].Recoverable {
		t.Fatalf("unproven v2 was mutated/cleared: %+v %v", after, err)
	}
	lease, err := runtime.Sandboxes.ActiveLeases(ctx, item.ID())
	if err != nil || lease != 0 {
		t.Fatalf("failed recovery leaked short fence: %d %v", lease, err)
	}
	if _, err = runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, owner); !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("unknown v2 record allowed lifecycle after failed Resolve: %v", err)
	}
}
