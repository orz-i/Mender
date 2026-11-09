package sqlite

import (
	"context"
	"fmt"

	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
)

// OpenTyped reuses the existing MCP Delivery Store on an already migrated
// v3 database. Unlike legacy Open, it neither migrates a Profile delivery
// nor installs legacy tunnel guards. The future single Runtime composer
// must validate Tunnel authority and owner-approved activation separately.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := hardcut.OpenTypedContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed MCP delivery: %w", err)
	}
	return &Store{db: db}, nil
}
