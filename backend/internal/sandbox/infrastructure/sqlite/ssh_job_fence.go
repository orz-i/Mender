package sqlite

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// SSHJobManaged is a local resource lifecycle obligation, NOT a Task table.
// Its absence means the host has never admitted the plugin Job submission
// path. Only this host-only admission path may enroll a Sandbox.
func (s *Store) SSHJobManaged(ctx context.Context, id sandboxdomain.ID) (bool, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return false, err
	}
	var found int
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sandbox_ssh_job_obligations WHERE sandbox_id=?)`, id.String()).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("SSH job obligation lookup: %w", err)
	}
	return found == 1, nil
}
func jobFenceToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
func verifyJobFenceCaller(caller gov.Context) error {
	if err := caller.Validate(); err != nil {
		return err
	}
	if caller.Principal().Kind() != gov.PrincipalLocalUser || caller.Scope().Kind() != gov.ScopeCatalog || caller.Origin() == gov.OriginMCP {
		return sandboxdomain.ErrNotAuthorized
	}
	switch caller.Origin() {
	case gov.OriginCLI, gov.OriginDesktop, gov.OriginManagementHTTP, gov.OriginInternal:
		return nil
	default:
		return sandboxdomain.ErrNotAuthorized
	}
}

// ReserveSSHJobSubmit is host-only and must follow owner and Policy admission.
// It serializes against Sandbox Workspace leases and lifecycle fences. Marking
// a permanent obligation in the SAME transaction occurs BEFORE remote side
// effects, so an uncertain submission can never escape lifecycle blocking.
// The acquired lease is short-lived and MUST be released after the SSH submit
// attempt (even if the result is uncertain); the plugin's remote record owns
// the ongoing execution and recovery status.
func (s *Store) ReserveSSHJobSubmit(ctx context.Context, id sandboxdomain.ID, generation uint64, caller gov.Context) (string, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return "", err
	}
	if generation == 0 {
		return "", sandboxdomain.ErrConflict
	}
	if err := verifyJobFenceCaller(caller); err != nil {
		return "", err
	}
	token, err := jobFenceToken()
	if err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	person := caller.Principal()
	scope := caller.Scope()
	result, err := tx.ExecContext(ctx, `INSERT INTO sandbox_execution_leases(
 sandbox_id,lease_token,generation,principal_kind,principal_ref,scope_kind,scope_ref,acquired_at)
 SELECT r.sandbox_id,?,?,?,?,?,?,? FROM sandbox_runs r
 JOIN sandboxes s ON s.id=r.sandbox_id WHERE r.sandbox_id=? AND r.generation=? AND r.state='ready'
 AND s.backend='ssh_remote' AND s.backend_plugin_id='io.mender.sandbox.ssh-remote'
 AND s.backend_installation_id='' AND s.owner_kind=? AND s.owner_ref=?`,
		token, generation, string(person.Kind()), person.Reference(), string(scope.Kind()), scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano),
		id.String(), generation, string(person.Kind()), person.Reference())
	if err != nil || !oneRow(result) {
		return "", sandboxdomain.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO sandbox_ssh_job_obligations(sandbox_id,enrolled_at)
 VALUES(?,?)`, id.String(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

// ReserveSSHJobLifecycle places the same exclusive DB lease used by all
// acquisitions, then the plugin checks for remote nonterminal/unknown Jobs.
// Its token must be atomically consumed with the approved Sandbox transition.
func (s *Store) ReserveSSHJobLifecycle(ctx context.Context, expected sandboxdomain.Run, caller gov.Context) (string, error) {
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
	person := caller.Principal()
	scope := caller.Scope()
	result, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_execution_leases(
 sandbox_id,lease_token,generation,principal_kind,principal_ref,scope_kind,scope_ref,acquired_at)
 SELECT r.sandbox_id,?,?,?,?,?,?,? FROM sandbox_runs r
 JOIN sandboxes s ON s.id=r.sandbox_id WHERE r.sandbox_id=? AND r.generation=? AND r.state=? AND r.external_id=?
 AND s.backend='ssh_remote' AND s.backend_plugin_id='io.mender.sandbox.ssh-remote'
 AND s.backend_installation_id='' AND s.owner_kind=? AND s.owner_ref=?
 AND EXISTS(SELECT 1 FROM sandbox_ssh_job_obligations j WHERE j.sandbox_id=s.id)`,
		token, expected.Generation, string(person.Kind()), person.Reference(), string(scope.Kind()), scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano),
		expected.SandboxID.String(), expected.Generation, string(expected.State), expected.ExternalID, string(person.Kind()), person.Reference())
	if err != nil || !oneRow(result) {
		return "", sandboxdomain.ErrConflict
	}
	return token, nil
}
func oneRow(result interface{ RowsAffected() (int64, error) }) bool {
	if result == nil {
		return false
	}
	n, err := result.RowsAffected()
	return err == nil && n == 1
}
