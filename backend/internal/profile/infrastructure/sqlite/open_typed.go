package sqlite

import (
	"context"
	"fmt"

	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

// OpenTyped is the only Profile Store constructor. It requires the pinned v3
// schema and never creates resources, grants or legacy migration tables.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := v3schema.OpenContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Profile sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
