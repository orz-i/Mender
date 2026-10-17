package sqlite

import (
	"context"
	"fmt"
)

// CurrentPolicyRevision is a strict read of the same SQLite Policy settings
// database that the Agent approval ledger writes into. A missing/old Policy
// schema cannot authorize an unattended ACP action.
func (s *Store) CurrentPolicyRevision(ctx context.Context) (int64, error) {
	var revision int64
	if err := s.db.QueryRowContext(ctx, "SELECT revision FROM policy_settings WHERE singleton_id=1").Scan(&revision); err != nil {
		return 0, fmt.Errorf("agent approval policy revision unavailable: %w", err)
	}
	if revision < 1 {
		return 0, fmt.Errorf("agent approval policy revision invalid")
	}
	return revision, nil
}
