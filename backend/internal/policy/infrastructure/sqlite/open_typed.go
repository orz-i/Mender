package sqlite

import (
	"context"
	"fmt"

	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

// OpenTyped is the only Policy Store constructor. It requires the pinned v3
// database schema and never migrates or resets existing Policy rules.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := v3schema.OpenContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Policy sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
