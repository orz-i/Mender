package sqlite

import (
	"context"
	"fmt"
)

// CatalogRevision changes atomically on every canonical Tool write.
func (s *Store) CatalogRevision(ctx context.Context) (int64, error) {
	var revision int64
	if err := s.db.QueryRowContext(ctx, "SELECT revision FROM catalog_tool_revision WHERE id=1").Scan(&revision); err != nil {
		return 0, fmt.Errorf("catalog revision: %w", err)
	}
	return revision, nil
}
