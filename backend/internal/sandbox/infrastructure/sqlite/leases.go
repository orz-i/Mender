package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// ReserveExecution atomically checks persisted Run generation/state and takes
// its unique lease. There is no lease TTL: a crashed executor requires reviewed
// reconciliation instead of silently allowing another writer or destroyer.
func (s *Store) ReserveExecution(ctx context.Context, id sandboxdomain.ID, generation uint64, principal governancedomain.Context, capability sandboxdomain.Capability) (string, error) {
	if s == nil || s.db == nil {
		return "", sandboxdomain.ErrBackendUnavailable
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return "", err
	}
	if generation == 0 {
		return "", sandboxdomain.ErrConflict
	}
	if err := principal.Validate(); err != nil {
		return "", err
	}
	if err := capability.Validate(); err != nil {
		return "", err
	}
	profileID, delegated := sandboxapplication.ProfileServiceID(principal)
	if !delegated && principal.Principal().Kind() != governancedomain.PrincipalLocalUser {
		// Preserve actual service-owned Sandbox access while rejecting untrusted
		// service-to-other-owner acquisition. The INSERT still checks ownership.
		item, err := s.Get(ctx, id)
		if err != nil {
			return "", err
		}
		if !sandboxapplication.IsOwner(principal, item) {
			return "", sandboxdomain.ErrNotAuthorized
		}
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("reserve Sandbox lease: random token: %w", err)
	}
	token := hex.EncodeToString(random[:])
	subject := principal.Principal()
	scope := principal.Scope()
	// Authorize and reserve as ONE SQLite write. The grant cannot be revoked
	// between authorization and the lease; other Runtime processes share this DB.
	delegatedFlag := 0
	if delegated {
		delegatedFlag = 1
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_execution_leases(sandbox_id,lease_token,generation,principal_kind,principal_ref,scope_kind,scope_ref,acquired_at)
 SELECT ?,?,?,?,?,?,?,? FROM sandbox_runs r
 JOIN sandboxes s ON s.id=r.sandbox_id
 WHERE r.sandbox_id=? AND r.generation=? AND r.state='ready'
 AND (
   (s.owner_kind=? AND s.owner_ref=?)
   OR (?=1 AND EXISTS (
     SELECT 1 FROM sandbox_profile_grants g
     WHERE g.sandbox_id=s.id AND g.profile_id=? AND g.capability=?
   ))
 )`, id.String(), token, generation, string(subject.Kind()), subject.Reference(), string(scope.Kind()), scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano), id.String(), generation, string(subject.Kind()), subject.Reference(), delegatedFlag, profileID, string(capability))
	if err != nil {
		if uniqueError(err) {
			return "", sandboxdomain.ErrConflict
		}
		return "", fmt.Errorf("reserve Sandbox lease: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("reserve Sandbox lease: rows: %w", err)
	}
	if count != 1 {
		return "", sandboxdomain.ErrConflict
	}
	return token, nil
}

// ReleaseExecution is idempotent only for the caller holding the right lease
// token. An absent or mismatched token cannot release another owner's lease.
func (s *Store) ReleaseExecution(ctx context.Context, id sandboxdomain.ID, token string) error {
	if s == nil || s.db == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return err
	}
	if len(token) != 32 {
		return fmt.Errorf("release Sandbox lease: invalid token")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return fmt.Errorf("release Sandbox lease: invalid token: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_execution_leases WHERE sandbox_id=? AND lease_token=?`, id.String(), token)
	if errors.Is(err, sql.ErrNoRows) {
		return sandboxdomain.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("release Sandbox lease: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("release Sandbox lease: rows: %w", err)
	}
	if count != 1 {
		return sandboxdomain.ErrConflict
	}
	return nil
}

// ActiveLeaseCount returns durable lease count, not an in-memory approximation.
func (s *Store) ActiveLeaseCount(ctx context.Context, id sandboxdomain.ID) (int, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return 0, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_execution_leases WHERE sandbox_id=?`, id.String()).Scan(&count); err != nil {
		return 0, fmt.Errorf("get active Sandbox leases: %w", err)
	}
	return count, nil
}
