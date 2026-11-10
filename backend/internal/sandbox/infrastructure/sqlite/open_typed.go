package sqlite

import (
	"context"
	"fmt"

	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

// OpenTyped is the sole Sandbox Store constructor. It requires an already
// validated v3 schema and performs no migration, backup, lease recovery, Job
// reattachment or remote probe. Those lifecycle decisions belong to Runtime.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := v3schema.OpenContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Sandbox sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
