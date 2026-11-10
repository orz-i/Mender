package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"

	_ "modernc.org/sqlite"
)

// Store persists data-minimized invocation audit records.
type Store struct {
	db *sql.DB
}

// Open verifies the current pinned v3 schema without migrating historic
// Audit records, creating resources or changing a terminal invocation.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := v3schema.OpenContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open Audit sqlite: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database pool.
func (s *Store) Close() error {
	return s.db.Close()
}
