package sqlite

import (
	"context"
	"fmt"

	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

// OpenTyped is the only MCP Delivery Store constructor. It strictly verifies
// the current v3 schema without migrating Profile delivery or installing old
// Tunnel guards. The Runtime separately validates live Tunnel activation.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := v3schema.OpenContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed MCP delivery: %w", err)
	}
	return &Store{db: db}, nil
}
