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
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// ReserveExecution atomically checks persisted Run generation/state and takes
// its unique lease. There is no lease TTL: a crashed executor requires reviewed
// reconciliation instead of silently allowing another writer or destroyer.
func (s *Store) ReserveExecution(ctx context.Context, id sandboxdomain.ID, generation uint64, principal governancedomain.Context) (string, error) {
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
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("reserve Sandbox lease: random token: %w", err)
	}
	token := hex.EncodeToString(random[:])
	subject := principal.Principal()
	scope := principal.Scope()
	result, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_execution_leases(sandbox_id,lease_token,generation,principal_kind,principal_ref,scope_kind,scope_ref,acquired_at)
 SELECT ?,?,?,?,?,?,?,? FROM sandbox_runs WHERE sandbox_id=? AND generation=? AND state='ready'`, id.String(), token, generation, string(subject.Kind()), subject.Reference(), string(scope.Kind()), scope.Reference(), time.Now().UTC().Format(time.RFC3339Nano), id.String(), generation)
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
