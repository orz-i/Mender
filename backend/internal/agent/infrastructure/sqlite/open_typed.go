package sqlite

import (
	"context"
	"fmt"

	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
)

// OpenTyped reuses the existing Agent/Task/Session/Grant Store on a verified
// v3 schema, WITHOUT legacy migrations or the legacy Open's automatic
// InvalidateAllPendingApprovals. F4 Runtime alone must coordinate Agent
// recovery and pending approvals after quiescence and task ownership review.
// It never starts ACP, replays unknown tasks, or changes pending approvals.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := hardcut.OpenTypedContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Agent sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
