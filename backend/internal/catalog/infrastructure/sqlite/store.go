package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Store implements Catalog application persistence ports with SQLite.
type Store struct {
	db *sql.DB
}

// Open opens or creates a Catalog SQLite database and applies migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open catalog sqlite: path must not be empty")
	}

	dsn, err := dataSourceName(path)
	if err != nil {
		return nil, fmt.Errorf("open catalog sqlite: %w", err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open catalog sqlite: %w", err)
	}

	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open catalog sqlite: ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open catalog sqlite: migrate: %w", err)
	}

	return store, nil
}

// Close closes the underlying database pool.
func (s *Store) Close() error {
	return s.db.Close()
}

func dataSourceName(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()

	return u.String(), nil
}
