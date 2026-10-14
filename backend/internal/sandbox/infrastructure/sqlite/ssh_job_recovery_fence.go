package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// ReserveSSHJobRecovery is the Core-owned, owner-only, short exclusive fence.
// It shares sandbox_execution_leases with Submit, Workspace and Stop/Destroy;
// it never stores backend Task state or uses a wall-clock TTL.
func (s *Store) ReserveSSHJobRecovery(ctx context.Context, expected sandboxdomain.Run, caller gov.Context) (string, error) {
	if s == nil || s.db == nil {
		return "", sandboxdomain.ErrBackendUnavailable
	}
	if _, err := sandboxdomain.NewRun(expected.SandboxID, expected.Generation, expected.State, expected.ExternalID); err != nil {
		return "", err
	}
	if expected.State != sandboxdomain.StateReady && expected.State != sandboxdomain.StateStopped {
		return "", sandboxdomain.ErrConflict
	}
	if err := verifyJobFenceCaller(caller); err != nil {
		return "", err
	}
	token, err := jobFenceToken()
	if err != nil {
		return "", err
	}
	subject := caller.Principal()
	scope := caller.Scope()
	result, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_execution_leases
    (sandbox_id,lease_token,generation,principal_kind,principal_ref,scope_kind,scope_ref,acquired_at)
 SELECT r.sandbox_id,?,?,?,?,?,?,?
 FROM sandbox_runs r JOIN sandboxes s ON s.id=r.sandbox_id
 WHERE r.sandbox_id=? AND r.generation=? AND r.state=? AND r.external_id=?
 AND s.backend='ssh_remote' AND s.backend_plugin_id='io.mender.sandbox.ssh-remote'
 AND s.backend_installation_id='' AND s.owner_kind=? AND s.owner_ref=?
 AND EXISTS(SELECT 1 FROM sandbox_ssh_job_obligations j WHERE j.sandbox_id=s.id)
 AND EXISTS(SELECT 1 FROM sandbox_ssh_job_admission_controls c WHERE c.sandbox_id=s.id AND c.paused=1)`,
		token, expected.Generation, string(subject.Kind()), subject.Reference(), string(scope.Kind()), scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano),
		expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID, string(subject.Kind()), subject.Reference())
	if err != nil || !oneRow(result) {
		return "", sandboxdomain.ErrConflict
	}
	return token, nil
}

type SSHRecoveryAuditInput struct {
	SandboxID   sandboxdomain.ID
	TaskID      string
	Fingerprint string
	Reason      string
	Generation  uint64
	Outcome     string
	Token       string
	Caller      gov.Context
}

// RecordSSHJobRecoveryAttempt retains a privacy-bounded decision: never raw
// command, log, host path or free-text reason. It must be associated with the
// exact held recovery fence or the resolution cannot be acknowledged.
func (s *Store) RecordSSHJobRecoveryAttempt(ctx context.Context, input SSHRecoveryAuditInput) error {
	if _, err := sandboxdomain.ParseID(input.SandboxID.String()); err != nil {
		return err
	}
	if len(input.TaskID) != 37 || !strings.HasPrefix(input.TaskID, "sjob_") ||
		len(input.Fingerprint) != 64 || len(input.Reason) < 5 ||
		len(input.Token) != 32 || input.Generation == 0 ||
		(input.Outcome != "resolved" && input.Outcome != "unknown" && input.Outcome != "conflict") {
		return sandboxdomain.ErrConflict
	}
	if err := verifyJobFenceCaller(input.Caller); err != nil {
		return err
	}
	principal := input.Caller.Principal()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var held int
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sandbox_execution_leases
 WHERE sandbox_id=? AND lease_token=? AND generation=? AND principal_kind=? AND principal_ref=?)`,
		input.SandboxID.String(), input.Token, input.Generation, string(principal.Kind()), principal.Reference()).Scan(&held)
	if err != nil || held != 1 {
		return sandboxdomain.ErrConflict
	}
	reason := sha256.Sum256([]byte(input.Reason))
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_ssh_job_recovery_events
  (sandbox_id,task_id,action,evidence_digest,reason_digest,outcome,generation,principal_kind,principal_ref,origin,occurred_at)
 VALUES(? ,? ,'aborted_before_launch',? ,? ,? ,? ,? ,? ,? ,?)`,
		input.SandboxID.String(), input.TaskID, input.Fingerprint, hex.EncodeToString(reason[:]), input.Outcome,
		input.Generation, string(principal.Kind()), principal.Reference(), string(input.Caller.Origin()), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("SSH recovery audit: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("SSH recovery audit commit: %w", err)
	}
	return nil
}

func (s *Store) SSHRecoveryAuditCount(ctx context.Context, id sandboxdomain.ID) (int, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return 0, err
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_ssh_job_recovery_events WHERE sandbox_id=?`, id.String()).Scan(&count)
	return count, err
}
