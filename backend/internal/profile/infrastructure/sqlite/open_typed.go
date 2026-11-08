package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// OpenTyped opens the existing Profile Store implementation against a
// previously reviewed v3 hard-cut database. It never creates a DB or invokes
// Profile migrations. This is an F4 composition prerequisite, not a migration
// command or an independent Profile authorization path.
//
// The old bootstrap Runtime still rejects typed databases before opening ANY
// Store. The final hard-cut composer must use only this existing Store's
// Profile operations with the typed Invocation/target-scope guards.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open typed Profile sqlite: database path required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open typed Profile sqlite: %w", err)
	}
	file, err := os.Lstat(absolute)
	if err != nil {
		return nil, fmt.Errorf("open typed Profile sqlite: %w", err)
	}
	if !file.Mode().IsRegular() {
		return nil, fmt.Errorf("open typed Profile sqlite: existing regular database required")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	q := u.Query()
	q.Set("mode", "rw") // Never create an absent DB or run legacy migrations.
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open typed Profile sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := verifyTypedProfileSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open typed Profile sqlite: %w", err)
	}
	return &Store{db: db}, nil
}

// The hard-cut marker AND cross-context versions must all agree. A partially
// converted DB must fail before any Profile operation, not be 'repaired' by
// running the Provider-v2 context migrator.
func verifyTypedProfileSchema(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	// Read one coherent pre-Store snapshot; a partial cross-context cut must
	// not pass by changing Schema versions halfway through verification.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, expected := range []struct {
		table string
		value int64
	}{
		{"tool_hardcut_schema_migrations", 1},
		{"catalog_schema_migrations", 3},
		{"profile_schema_migrations", 8},
		{"policy_schema_migrations", 4},
		{"agent_schema_migrations", 20},
		{"sandbox_schema_migrations", 20},
		{"provider_config_schema_migrations", 5},
		{"mcp_delivery_schema_migrations", 2},
	} {
		var version sql.NullInt64
		// All SQL identifiers are fixed, reviewed source constants.
		if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM "+expected.table).Scan(&version); err != nil {
			return fmt.Errorf("required hard-cut schema %s: %w", expected.table, err)
		}
		if !version.Valid || version.Int64 != expected.value {
			return fmt.Errorf("required hard-cut schema %s version %d", expected.table, expected.value)
		}
	}
	for _, query := range []string{
		"SELECT source_kind,source_id FROM tools LIMIT 0",
		"SELECT profile_id,tool_id,workspace_id,sandbox_id FROM profile_tool_target_scopes LIMIT 0",
		"SELECT workspace_id,profile_id FROM workspace_profile_grants LIMIT 0",
		"SELECT sandbox_id,profile_id FROM sandbox_profile_grants LIMIT 0",
		"SELECT agent_id,profile_id FROM agent_grants LIMIT 0",
		"SELECT tool_id FROM native_tool_contracts LIMIT 0",
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("required hard-cut Profile authority: %w", err)
		}
	}
	// The old facade tables must be retired in a completely cut candidate.
	for _, old := range []string{
		"agent_tool_facades", "sandbox_tool_suite_bindings", "sandbox_job_suite_bindings", "workspace_tool_targets",
	} {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", old).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("legacy Profile-owned Tool facade still present")
		}
	}
	return tx.Commit()
}
