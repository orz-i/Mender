package sqlite

import (
	"context"
	"fmt"

	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
)

// OpenTyped reuses existing Sandbox/Lease/SSH Job persistence on an already
// upgraded v3 schema. Unlike legacy Open it performs NO pre-upgrade backup or
// per-context migration, no lease recovery, no Job reattachment and no remote
// probe. F4 Runtime must do all release and lifecycle admission separately.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := hardcut.OpenTypedContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Sandbox sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
