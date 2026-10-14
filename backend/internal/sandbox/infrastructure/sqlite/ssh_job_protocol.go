package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const (
	SSHJobProtocolLegacy = 1
	SSHJobProtocolV2     = 2
)

// SSHJobProtocol is Core-owned, immutable admission authority. A missing row
// means a not-yet-activated legacy Sandbox; v2 activation is a one-way change
// for FUTURE submissions and never rewrites existing v1 TaskIDs.
func (s *Store) SSHJobProtocol(ctx context.Context, id sandboxdomain.ID) (int, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return 0, err
	}
	var protocol int
	err := s.db.QueryRowContext(ctx, `SELECT protocol FROM sandbox_ssh_job_protocols WHERE sandbox_id=?`, id.String()).Scan(&protocol)
	if errors.Is(err, sql.ErrNoRows) {
		return SSHJobProtocolLegacy, nil
	}
	if err != nil {
		return 0, fmt.Errorf("SSH Job protocol lookup: %w", err)
	}
	if protocol != SSHJobProtocolV2 {
		return 0, sandboxdomain.ErrConflict
	}
	return protocol, nil
}

// EnableSSHJobV2 requires an explicitly paused, exact owner-bound Ready Run,
// with NO in-flight Sandbox execution lease. The remote capability check MUST
// have succeeded beforehand through the trusted, pinned SSH Backend. This
// transaction never contacts SSH or creates a TaskID.
// The immutable v2 row and non-sensitive activation audit commit together.
func (s *Store) EnableSSHJobV2(ctx context.Context, expected sandboxdomain.Run, caller gov.Context) error {
	if _, err := sandboxdomain.NewRun(expected.SandboxID, expected.Generation, expected.State, expected.ExternalID); err != nil {
		return err
	}
	if expected.State != sandboxdomain.StateReady {
		return sandboxdomain.ErrConflict
	}
	if err := verifyJobFenceCaller(caller); err != nil {
		return err
	}
	for attempt := 0; attempt < 6; attempt++ {
		err := s.enableSSHJobV2Once(ctx, expected, caller)
		if err == nil || !sqliteBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 15 * time.Millisecond):
		}
	}
	return fmt.Errorf("SSH Job v2 activation contention: %w", sandboxdomain.ErrConflict)
}
func (s *Store) enableSSHJobV2Once(ctx context.Context, expected sandboxdomain.Run, caller gov.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	principal := caller.Principal()
	var valid int
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM sandbox_runs r JOIN sandboxes s ON s.id=r.sandbox_id
 WHERE r.sandbox_id=? AND r.generation=? AND r.state=? AND r.external_id=?
 AND s.backend='ssh_remote' AND s.backend_plugin_id='io.mender.sandbox.ssh-remote'
 AND s.backend_installation_id='' AND s.owner_kind=? AND s.owner_ref=?
 AND EXISTS(SELECT 1 FROM sandbox_ssh_job_admission_controls c WHERE c.sandbox_id=r.sandbox_id AND c.paused=1)
 AND NOT EXISTS(SELECT 1 FROM sandbox_execution_leases l WHERE l.sandbox_id=r.sandbox_id)
 )`, expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID, string(principal.Kind()), principal.Reference()).Scan(&valid)
	if err != nil {
		return err
	}
	if valid != 1 {
		return sandboxdomain.ErrConflict
	}
	var prior int
	err = tx.QueryRowContext(ctx, `SELECT protocol FROM sandbox_ssh_job_protocols WHERE sandbox_id=?`, expected.SandboxID.String()).Scan(&prior)
	if err == nil {
		if prior != SSHJobProtocolV2 {
			return sandboxdomain.ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_ssh_job_protocols(sandbox_id,protocol,generation,activated_at) VALUES(?,2,?,?)`,
		expected.SandboxID.String(), expected.Generation, now)
	if err != nil {
		return fmt.Errorf("activate SSH v2 protocol: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_ssh_job_protocol_events(
 sandbox_id,action,generation,principal_kind,principal_ref,origin,occurred_at)
 VALUES(?,'enable_v2',?,?,?,?,?)`,
		expected.SandboxID.String(), expected.Generation, string(principal.Kind()), principal.Reference(), string(caller.Origin()), now)
	if err != nil {
		return fmt.Errorf("audit SSH v2 activation: %w", err)
	}
	return tx.Commit()
}
