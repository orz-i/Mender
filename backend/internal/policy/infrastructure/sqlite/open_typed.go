package sqlite

import (
	"context"
	"fmt"

	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
)

// OpenTyped returns the same Policy Store on a fully migrated v3 database.
// It never runs legacy Policy migrations or silently resets policy rules.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := hardcut.OpenTypedContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Policy sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
