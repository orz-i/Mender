package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// The Provider-v2 Runtime must never touch an F4 converted database.
// Inspect metadata BEFORE a writable Store, Task reconciliation, or activation.
// F4 replaces this guard when one typed Runtime owns all migrated contexts.
func requireLegacyRuntimeSchema(ctx context.Context, databasePath string) error {
	absolute, err := filepath.Abs(databasePath)
	if err != nil {
		return fmt.Errorf("compose runtime: resolve database: %w", err)
	}
	_, err = os.Stat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		// A new absent DB remains supported by the current v2 Runtime only.
		return nil
	}
	if err != nil {
		return fmt.Errorf("compose runtime: inspect database: %w", err)
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	q := uri.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return fmt.Errorf("compose runtime: inspect schema: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	var marker int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='tool_hardcut_schema_migrations'`).Scan(&marker); err != nil {
		return fmt.Errorf("compose runtime: inspect hard-cut marker: %w", err)
	}
	if marker > 0 {
		return errors.New("compose runtime: typed Tool hard-cut database requires F4 Runtime; Provider-v2 cannot start")
	}
	versions := []struct {
		table string
		max   int
	}{
		{"catalog_schema_migrations", 2},
		{"profile_schema_migrations", 7},
		{"policy_schema_migrations", 3},
		{"agent_schema_migrations", 19},
		{"sandbox_schema_migrations", 19},
		{"provider_config_schema_migrations", 4},
		{"mcp_delivery_schema_migrations", 1},
	}
	for _, v := range versions {
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
			WHERE type='table' AND name=?`, v.table).Scan(&exists); err != nil {
			return fmt.Errorf("compose runtime: inspect schema versions: %w", err)
		}
		if exists == 0 {
			continue
		}
		var current sql.NullInt64
		// #nosec G202 -- v.table is from a fixed local list.
		if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM "+v.table).Scan(&current); err != nil {
			return fmt.Errorf("compose runtime: inspect %s: %w", v.table, err)
		}
		if current.Valid && current.Int64 > int64(v.max) {
			return fmt.Errorf("compose runtime: %s version %d newer than Provider-v2 supported %d; refusing before writes",
				v.table, current.Int64, v.max)
		}
	}
	// An altered/missing marker cannot let typed Tool rows fall into v2.
	var toolsTable int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='tools'`).Scan(&toolsTable); err != nil {
		return fmt.Errorf("compose runtime: inspect Tool table: %w", err)
	}
	if toolsTable != 0 {
		rows, err := db.QueryContext(ctx, "PRAGMA table_info(tools)")
		if err != nil {
			return fmt.Errorf("compose runtime: inspect Tool identity: %w", err)
		}
		var hasTypedSource bool
		for rows.Next() {
			var seq, notNull, primaryKey int
			var name, columnType string
			var defaultValue sql.NullString
			if err := rows.Scan(&seq, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				_ = rows.Close()
				return fmt.Errorf("compose runtime: inspect Tool identity: %w", err)
			}
			if name == "source_kind" || name == "source_id" {
				hasTypedSource = true
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return fmt.Errorf("compose runtime: inspect Tool identity: %w", err)
		}
		if hasTypedSource {
			return errors.New("compose runtime: typed Tool source columns found without hard-cut marker; refusing Provider-v2 startup")
		}
	}
	return nil
}
