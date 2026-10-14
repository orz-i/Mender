package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

// auditFailureCredentialSpy rejects every credential lookup. If a missing
// local intent-audit table permits a subsequent SSH dial, the test observes
// that unsafe attempt without contacting an actual external SSH host.
type auditFailureCredentialSpy struct{ calls int }

func (s *auditFailureCredentialSpy) Resolve(context.Context, credentialdomain.Reference) ([]byte, error) {
	s.calls++
	return nil, errors.New("unexpected credential lookup after audit failure")
}

func TestSSHRecoveryAuditIntentCommitsBeforeAnyRemoteContact(t *testing.T) {
	ctx := context.Background()
	template, secret, root := startInProcessSSHForSandbox(t)
	template.User = "root"
	databasePath := filepath.Join(t.TempDir(), "r3b-preflight-audit.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: databasePath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	runtime.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := gov.NewContext(gov.OriginInternal, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	resource, err := runtime.Sandboxes.CreateSSHRemoteSandbox(ctx,
		CreateSSHRemoteSandboxRequest{Name: "R3 audit preflight fixture", Template: template}, owner)
	if err != nil {
		t.Fatal(err)
	}
	enrollSSHJobV2Fixture(t, ctx, runtime, resource.ID(), owner)
	testJob := seedSSHJobV2(t, root, resource.ID(), "sjob_77777777777777777777777777777777", false)
	view, err := runtime.Sandboxes.InspectSSHJobRecovery(ctx, resource.ID(), owner)
	if err != nil || len(view.Evidence.Records) != 1 || !view.Evidence.Records[0].Recoverable {
		t.Fatalf("native v2 prelaunch evidence unavailable: %+v %v", view, err)
	}
	if _, err = runtime.Sandboxes.SetSSHJobSubmissionsPaused(ctx, resource.ID(), true, owner); err != nil {
		t.Fatal(err)
	}

	// Only the disposable test DB is modified. The required intent audit
	// destination is made unavailable to inject local persistence failure.
	raw, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.ExecContext(ctx, "DROP TABLE sandbox_ssh_job_recovery_events"); err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	spy := &auditFailureCredentialSpy{}
	runtime.Sandboxes.sshRemote.Dial.Credentials = spy
	_, err = runtime.Sandboxes.ResolveSSHJobRecovery(ctx, resource.ID(), SSHJobResolutionRequest{
		TaskID: testJob.ID, Generation: view.Generation,
		Fingerprint: view.Evidence.Records[0].Fingerprint,
		Reason:      "verified evidence requires owner-only recovery",
	}, owner)
	if !errors.Is(err, sshremote.ErrJobUnknown) {
		t.Fatalf("failed audit did not block resolution: %v", err)
	}
	if spy.calls != 0 {
		t.Fatalf("remote Credential resolved before recovery audit committed: %d", spy.calls)
	}
	active, err := runtime.Sandboxes.ActiveLeases(ctx, resource.ID())
	if err != nil || active != 0 {
		t.Fatalf("failed preflight audit leaked recovery lease: %d %v", active, err)
	}
}
