package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestSSHRecoveryFenceRequiresPausedOwnedGenerationAndTracksAudit(t *testing.T) {
	filename, id, run, owner := newFencedSSHFixture(t)
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
	if _, err = first.ReserveSSHJobRecovery(ctx, run, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unmanaged unpaused sandbox acquired recovery fence: %v", err)
	}
	token, err := first.ReserveSSHJobSubmit(ctx, id, run.Generation, owner, SSHJobProtocolLegacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.ReleaseExecution(ctx, id, token); err != nil {
		t.Fatal(err)
	}
	if _, err = first.ReserveSSHJobRecovery(ctx, run, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unpaused sandbox acquired recovery fence: %v", err)
	}
	if err = first.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, true, owner); err != nil {
		t.Fatal(err)
	}
	otherPrincipal, _ := gov.NewPrincipal(gov.PrincipalLocalUser, "os:other")
	other, _ := gov.NewContext(gov.OriginCLI, otherPrincipal, gov.CatalogScope())
	if _, err = second.ReserveSSHJobRecovery(ctx, run, other); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("other principal acquired recovery fence: %v", err)
	}
	recovery, err := second.ReserveSSHJobRecovery(ctx, run, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.ReserveSSHJobSubmit(ctx, id, run.Generation, owner, SSHJobProtocolLegacy); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("submit bypassed recovery: %v", err)
	}
	if _, err = first.ReserveSSHJobLifecycle(ctx, run, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("Stop/Destroy bypassed recovery: %v", err)
	}
	if err = first.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, false, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("resume bypassed active recovery: %v", err)
	}
	fingerprint := strings.Repeat("a", 64)
	input := SSHRecoveryAuditInput{SandboxID: id, TaskID: "sjob_" + strings.Repeat("f", 32),
		Fingerprint: fingerprint, Reason: "approved prelaunch validation", Generation: run.Generation,
		Token: recovery, Caller: owner, Outcome: "resolved"}
	if err = first.RecordSSHJobRecoveryAttempt(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err = first.RecordSSHJobRecoveryAttempt(ctx, SSHRecoveryAuditInput{
		SandboxID: id, TaskID: input.TaskID, Fingerprint: fingerprint, Reason: "x", Generation: run.Generation,
		Token: strings.Repeat("0", 32), Caller: owner, Outcome: "resolved",
	}); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("wrong token recorded audit: %v", err)
	}
	var count int
	err = first.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_ssh_job_recovery_events WHERE sandbox_id=? AND evidence_digest=? AND outcome='resolved' AND reason_digest NOT LIKE '%approved%'`, id.String(), fingerprint).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("missing minimal audit: %d %v", count, err)
	}
	if err = second.ReleaseExecution(ctx, id, recovery); err != nil {
		t.Fatal(err)
	}
	if err = second.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, false, owner); err != nil {
		t.Fatal(err)
	}
}
