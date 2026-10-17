package sqlite

import (
	"context"
	"fmt"
)

// CurrentRevision returns the trusted, persistent Policy generation. Agent
// approval admission must snapshot this BEFORE evaluating the Policy and
// include it in the final durable approval transaction.
func (s *Store) CurrentRevision(ctx context.Context) (int64, error) {
	var revision int64
	if err := s.db.QueryRowContext(ctx, "SELECT revision FROM policy_settings WHERE singleton_id=1").Scan(&revision); err != nil {
		return 0, fmt.Errorf("read Policy revision: %w", err)
	}
	if revision < 1 {
		return 0, fmt.Errorf("Policy revision must be positive")
	}
	return revision, nil
}
