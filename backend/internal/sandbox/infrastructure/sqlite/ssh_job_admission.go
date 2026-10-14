package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sqliteerr "modernc.org/sqlite"
)

// SSHJobSubmissionsPaused is host-owned admission state, not task state.
// A missing row (including a Sandbox with no Jobs history) means unpaused.
func (s *Store) SSHJobSubmissionsPaused(ctx context.Context, id sandboxdomain.ID) (bool, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return false, err
	}
	var paused int
	err := s.db.QueryRowContext(ctx, `SELECT paused FROM sandbox_ssh_job_admission_controls WHERE sandbox_id=?`, id.String()).Scan(&paused)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("SSH Job admission state: %w", err)
	}
	return paused == 1, nil
}

// SetSSHJobSubmissionsPaused is an exact owner and Run generation CAS in a
// single SQLite transaction, coupled to an immutable admission-change event.
// A Paused flag is tested in ReserveSSHJobSubmit's atomic INSERT. This cannot
// cancel a remote job that was admitted before the pause committed.
func (s *Store) SetSSHJobSubmissionsPaused(ctx context.Context, id sandboxdomain.ID, expectedGeneration uint64, paused bool, caller gov.Context) error {
	// Deferred SQLite transactions can fail upgrading a read snapshot to a
	// writer when a concurrent Submit reserves its lease. Retry the *whole*
	// CAS transaction with a fresh snapshot, never its individual statements.
	for attempt := 0; attempt < 6; attempt++ {
		err := s.setSSHJobSubmissionsPausedOnce(ctx, id, expectedGeneration, paused, caller)
		if err == nil || !sqliteBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 15 * time.Millisecond):
		}
	}
	return fmt.Errorf("SSH Job pause contention: %w", sandboxdomain.ErrConflict)
}
func sqliteBusy(err error) bool {
	var e *sqliteerr.Error
	return errors.As(err, &e) && e.Code()&0xff == 5
}
func (s *Store) setSSHJobSubmissionsPausedOnce(ctx context.Context, id sandboxdomain.ID, expectedGeneration uint64, paused bool, caller gov.Context) error {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return err
	}
	if expectedGeneration == 0 {
		return sandboxdomain.ErrConflict
	}
	if err := verifyJobFenceCaller(caller); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var generation uint64
	var state, kind, pluginID, installationID, ownerKind, ownerRef string
	err = tx.QueryRowContext(ctx, `SELECT r.generation,r.state,s.backend,s.backend_plugin_id,s.backend_installation_id,s.owner_kind,s.owner_ref
      FROM sandboxes s JOIN sandbox_runs r ON s.id=r.sandbox_id WHERE s.id=?`, id.String()).
		Scan(&generation, &state, &kind, &pluginID, &installationID, &ownerKind, &ownerRef)
	if errors.Is(err, sql.ErrNoRows) {
		return sandboxdomain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if generation != expectedGeneration || kind != "ssh_remote" || pluginID != "io.mender.sandbox.ssh-remote" || installationID != "" ||
		ownerKind != string(caller.Principal().Kind()) || ownerRef != caller.Principal().Reference() {
		return sandboxdomain.ErrConflict
	}
	if state != "ready" && state != "stopped" {
		return sandboxdomain.ErrConflict
	}
	if !paused && state != "ready" {
		return sandboxdomain.ErrConflict
	}
	// A recovery fence must not be bypassed by resuming submissions in a
	// second Runtime while the remote decision is still in flight.
	if !paused {
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_execution_leases WHERE sandbox_id=?`, id.String()).Scan(&active); err != nil {
			return err
		}
		if active != 0 {
			return sandboxdomain.ErrConflict
		}
	}

	var current int
	err = tx.QueryRowContext(ctx, `SELECT paused FROM sandbox_ssh_job_admission_controls WHERE sandbox_id=?`, id.String()).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if (err == nil && (current == 1) == paused) || (errors.Is(err, sql.ErrNoRows) && !paused) {
		return tx.Commit()
	}
	flag := 0
	action := "resume"
	if paused {
		flag = 1
		action = "pause"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_ssh_job_admission_controls(sandbox_id,paused,revision,updated_at)
    VALUES(?,?,1,?) ON CONFLICT(sandbox_id) DO UPDATE SET
      paused=excluded.paused,revision=revision+1,updated_at=excluded.updated_at`,
		id.String(), flag, now)
	if err != nil {
		return fmt.Errorf("update SSH Job admission: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_ssh_job_admission_events(sandbox_id,action,generation,principal_kind,principal_ref,origin,occurred_at)
       VALUES(?,?,?,?,?,?,?)`, id.String(), action, expectedGeneration, string(caller.Principal().Kind()), caller.Principal().Reference(), string(caller.Origin()), now)
	if err != nil {
		return fmt.Errorf("audit SSH Job pause: %w", err)
	}
	return tx.Commit()
}
