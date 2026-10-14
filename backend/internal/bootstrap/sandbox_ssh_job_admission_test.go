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

func TestSSHJobOwnerPausePreventsNetworkBeforeSubmissionAcrossRestart(t *testing.T) {
	ctx := context.Background()
	tpl, secret, _ := startInProcessSSHForSandbox(t)
	tpl.User = "root"
	db := filepath.Join(t.TempDir(), "jobs-pause.db")
	first, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	first.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, _ := gov.NewContext(gov.OriginInternal, first.OwnerPrincipal(), gov.CatalogScope())
	item, err := first.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "SSH Pause", Template: tpl}, owner)
	if err != nil {
		t.Fatal(err)
	}
	state, err := first.Sandboxes.SetSSHJobSubmissionsPaused(ctx, item.ID(), true, owner)
	if err != nil || !state.Paused {
		t.Fatalf("owner pause %+v %v", state, err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.Close() }()
	// No valid SSH Credential is wired after restart. A paused submission
	// MUST fail before any SSH handshake or secret resolution.
	newOwner, _ := gov.NewContext(gov.OriginInternal, next.OwnerPrincipal(), gov.CatalogScope())
	paused, err := next.Sandboxes.SSHJobAdmission(ctx, item.ID(), newOwner)
	if err != nil || !paused.Paused {
		t.Fatalf("restart forgot pause: %+v %v", paused, err)
	}
	_, err = next.Sandboxes.SubmitSSHJob(ctx, item.ID(), newOwner, sshremote.JobRequest{IdempotencyKey: strings.Repeat("e", 32), Command: "echo never"})
	if !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("paused attempt contacted backend: %v", err)
	}
	inspection, err := next.Sandboxes.InspectSSHJobRecovery(ctx, item.ID(), newOwner)
	if err != nil || !inspection.Paused || inspection.Managed {
		t.Fatalf("read-only pause state lost: %+v %v", inspection, err)
	}
	resumed, err := next.Sandboxes.SetSSHJobSubmissionsPaused(ctx, item.ID(), false, newOwner)
	if err != nil || resumed.Paused {
		t.Fatalf("resume failed: %+v %v", resumed, err)
	}
}
