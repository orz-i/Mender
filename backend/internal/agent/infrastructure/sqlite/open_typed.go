package sqlite

import (
	"context"
	"fmt"

	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

// OpenTyped is the only Agent Store constructor. The shared v3 schema must
// already be present and verified; startup recovery/approval invalidation is
// explicitly coordinated by the Runtime after Task ownership review. This
// Store constructor never starts ACP, replays Tasks or changes approvals.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := v3schema.OpenContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Agent sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
