package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

func TestSSHJobV2ActivationFailsClosedWhenRemoteLacksDurability(t *testing.T) {
	ctx := context.Background()
	tpl, secret, _ := startInProcessSSHForSandbox(t)
	tpl.User = "root"
	db := filepath.Join(t.TempDir(), "permanent-v2-activation.db")
	first, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	first.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := gov.NewContext(gov.OriginInternal, first.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := first.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "v2 capability failure fixture", Template: tpl}, owner)
	if err != nil {
		t.Fatal(err)
	}
	before, err := first.Sandboxes.SSHJobAdmission(ctx, item.ID(), owner)
	if err != nil || before.Protocol != "legacy_v1" || before.Paused {
		t.Fatalf("incorrect initial protocol: %+v %v", before, err)
	}
	if _, err = first.Sandboxes.ActivateSSHJobV2(ctx, item.ID(), owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unpaused activation contacted SSH: %v", err)
	}
	paused, err := first.Sandboxes.SetSSHJobSubmissionsPaused(ctx, item.ID(), true, owner)
	if err != nil || !paused.Paused || paused.Protocol != "legacy_v1" {
		t.Fatalf("pause: %+v %v", paused, err)
	}
	// The in-process SSH test server supports SFTP but does NOT offer the real
	// OpenSSH fsync extension, shell or util-linux flock; v2 activation must
	// refuse it, without falling back or persisting a partial v2 selection.
	if _, err = first.Sandboxes.ActivateSSHJobV2(ctx, item.ID(), owner); !errors.Is(err, sshremote.ErrJobUnavailable) {
		t.Fatalf("unsupported host activated v2: %v", err)
	}
	protocol, err := first.Sandboxes.store.SSHJobProtocol(ctx, item.ID())
	if err != nil || protocol != sandboxsqlite.SSHJobProtocolLegacy {
		t.Fatalf("failed check changed persisted protocol: %d %v", protocol, err)
	}
	state, err := first.Sandboxes.SSHJobAdmission(ctx, item.ID(), owner)
	if err != nil || !state.Paused || state.Protocol != "legacy_v1" {
		t.Fatalf("failed activation auto-resumed: %+v %v", state, err)
	}
	managed, err := first.Sandboxes.store.SSHJobManaged(ctx, item.ID())
	if err != nil || managed {
		t.Fatalf("activation reserved a TaskID/obligation: %t %v", managed, err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	newOwner, _ := gov.NewContext(gov.OriginInternal, reopened.OwnerPrincipal(), gov.CatalogScope())
	persisted, err := reopened.Sandboxes.SSHJobAdmission(ctx, item.ID(), newOwner)
	if err != nil || persisted.Protocol != "legacy_v1" || !persisted.Paused {
		t.Fatalf("failed remote preflight left bad state after restart: %+v %v", persisted, err)
	}
}
