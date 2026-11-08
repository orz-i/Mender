package sqlite

import (
	"context"
	"fmt"

	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
)

// OpenTyped reuses the existing Profile Store on a pre-migrated v3 database.
// No migration or grant creation occurs. The single F4 Runtime composer must
// perform owner-reviewed cutover admission before activating any execution.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := hardcut.OpenTypedContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Profile sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
