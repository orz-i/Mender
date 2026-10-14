//go:build liveacceptance

package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

// Explicit opt-in ONLY; excluded from regular Go tests and pnpm check.
// This is authorized operator acceptance on the pinned Rocky target.
// Never touches a preexisting TaskID. Creates only a new Sandbox-scoped
// TaskID/manifest, and preserves failures in quarantine without force clear.
func TestR3bRealSSHLiveAcceptance(t *testing.T) {
	if os.Getenv("MENDER_SSH_LIVE_ACCEPTANCE") != "enabled" {
		t.Skip("operator must explicitly enable both live acceptance and v2")
	}
	db := strings.TrimSpace(os.Getenv("MENDER_DB"))
	if db == "" {
		t.Fatal("MENDER_DB must be an explicit operator-approved database path")
	}
	originalID, err := sandboxdomain.ParseID(os.Getenv("MENDER_SSH_LIVE_TARGET_ID"))
	if err != nil {
		t.Fatal("missing or invalid explicitly approved Rocky Sandbox ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	caller, err := gov.NewContext(gov.OriginCLI, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	template, err := r.Sandboxes.store.GetSSHRemoteTemplate(ctx, originalID)
	if err != nil {
		t.Fatal(err)
	}
	if template.User != "root" {
		t.Fatal("SSH Jobs v2 requires exact verified root SSH template")
	}
	// Preflight is read-only except for a scoped fsync of the existing private
	// task directory; no user command, TaskID reservation, or systemd start.
	err = r.Sandboxes.sshRemote.WithJobs(ctx, originalID, func(j *sshremote.Jobs) error {
		return j.CheckV2(ctx)
	})
	if err != nil {
		t.Fatalf("REAL_ROCKY_PREFLIGHT_UNAVAILABLE (no mutation): %v", err)
	}
	t.Log("REAL_ROCKY_PREFLIGHT_PASS: SFTP fsync@openssh.com and pinned-host Linux/fsync/flock checks")
	if os.Getenv("MENDER_SSH_LIVE_STAGE") == "preflight" {
		return
	}
	stage := os.Getenv("MENDER_SSH_LIVE_STAGE")
	if stage != "smoke" && stage != "setup-recovery" {
		t.Skip("live side effects require explicit MENDER_SSH_LIVE_STAGE=smoke or setup-recovery")
	}
	// A brand-new logical Sandbox isolates the test TaskID directory from all
	// user jobs. Never target or retry a preexisting TaskID.
	owned, err := r.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{
		Name: "R3b Rocky acceptance temporary", Template: template,
	}, caller)
	if err != nil {
		t.Fatalf("cannot create scoped acceptance Sandbox: %v", err)
	}
	id := owned.ID()
	t.Logf("LIVE_ACCEPTANCE_SANDBOX_ID=%s (dedicated task namespace)", id)
	// All SSH Job submissions now use the v2 execution-fenced protocol.
	var key [16]byte
	if _, err = rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}
	stable := hex.EncodeToString(key[:])
	command := "printf 'mender-r3b-rocky-real-acceptance\\n'"
	submitted, err := r.Sandboxes.SubmitSSHJob(ctx, id, caller, sshremote.JobRequest{
		IdempotencyKey: stable, Command: command,
	})
	t.Logf("LIVE_V2_TASK_ID=%s; initial_state=%s; submit_err=%v", submitted.TaskID, submitted.State, err)
	if err != nil || submitted.TaskID == "" {
		t.Fatalf("live v2 submission unconfirmed, safe quarantine Sandbox=%s TaskID=%s: %v", id, submitted.TaskID, err)
	}
	var terminal sshremote.JobSnapshot
	deadline := time.NewTimer(105 * time.Second)
	defer deadline.Stop()
poll:
	for {
		current, e := r.Sandboxes.SSHJobStatus(ctx, id, caller, submitted.TaskID)
		if e == nil && current.State.Terminal() {
			terminal = current
			break poll
		}
		select {
		case <-deadline.C:
			t.Fatalf("live v2 task still unproven; do not auto-retry: sandbox=%s TaskID=%s last=%+v err=%v", id, submitted.TaskID, current, e)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	if terminal.State != sshremote.JobCompleted || terminal.ExitCode == nil || *terminal.ExitCode != 0 {
		t.Fatalf("live v2 task did not exit cleanly: %+v; sandbox=%s", terminal, id)
	}
	t.Logf("LIVE_V2_TERMINAL_RESULT=%s exit=0", terminal.State)
	stdout, err := r.Sandboxes.SSHJobLogs(ctx, id, caller, submitted.TaskID, "stdout", 0, 1024)
	if err != nil || stdout.Content != "mender-r3b-rocky-real-acceptance\n" {
		t.Fatalf("live v2 stdout mismatch: err=%v count=%d; sandbox=%s", err, len(stdout.Content), id)
	}
	after, err := r.Sandboxes.SubmitSSHJob(ctx, id, caller, sshremote.JobRequest{
		IdempotencyKey: stable, Command: command,
	})
	if err != nil || after.TaskID != submitted.TaskID || after.State != sshremote.JobCompleted {
		t.Fatalf("idempotent v2 retry attempted side effects: %+v %v; sandbox=%s", after, err, id)
	}
	observed, err := r.Sandboxes.InspectSSHJobRecovery(ctx, id, caller)
	if err != nil || len(observed.Evidence.Records) != 1 || observed.Evidence.Verdict != "quiescent_proved" ||
		observed.Evidence.Records[0].State != sshremote.JobCompleted {
		t.Fatalf("live backend evidence incorrect: %+v %v; sandbox=%s", observed, err, id)
	}
	if stage == "setup-recovery" {
		t.Logf("LIVE_RECOVERY_SETUP_READY: sandbox=%s generation=1; one completed v2 TaskID enrolled the lifecycle obligation; intentionally left Ready for isolated prelaunch fault injection", id)
		return
	}
	if _, err = r.Sandboxes.SetSSHJobSubmissionsPaused(ctx, id, true, caller); err != nil {
		t.Fatal(err)
	}
	var newKey [16]byte
	if _, err = rand.Read(newKey[:]); err != nil {
		t.Fatal(err)
	}
	_, err = r.Sandboxes.SubmitSSHJob(ctx, id, caller, sshremote.JobRequest{
		IdempotencyKey: hex.EncodeToString(newKey[:]), Command: "printf 'must-not-run\\n'",
	})
	if !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("paused submission passed admission; sandbox=%s err=%v", id, err)
	}
	t.Log("LIVE_V2_PAUSE_BLOCKED_NEW_SUBMIT")
	stopped, err := r.Sandboxes.ChangeLifecycle(ctx, id, gov.ActionSandboxStop, caller)
	if err != nil || stopped.State != sandboxdomain.StateStopped {
		t.Fatalf("live Stop not proved: sandbox=%s state=%+v err=%v", id, stopped, err)
	}
	destroyed, err := r.Sandboxes.ChangeLifecycle(ctx, id, gov.ActionSandboxDestroy, caller)
	if err != nil || destroyed.State != sandboxdomain.StateDestroyed {
		t.Fatalf("live Destroy not proved: sandbox=%s state=%+v err=%v", id, destroyed, err)
	}
	t.Logf("LIVE_V2_SMOKE_PASS: task=%s sandbox=%s (destroyed logical binding; remote tombstone retained)", submitted.TaskID, id)
}
