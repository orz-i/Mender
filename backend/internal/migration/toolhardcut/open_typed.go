package toolhardcut

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// OpenTypedContextStore returns a new SQLite handle for an existing, already
// migrated v3 database. It NEVER runs migrations, creates a missing file,
// activates a resource, or performs Agent/Sandbox startup recovery. F4's
// single runtime composer owns the remaining lifecycle admission.
//
// This preflight only verifies schema shape. It is NOT the owner-reviewed
// backup/cutover attestation from VerifyStagedCopy and not a grant to invoke.
func OpenTypedContextStore(ctx context.Context, path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("typed context store: database path required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("typed context store: existing regular database required")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	q := u.Query()
	q.Set("mode", "rw") // Do not create/repair a DB during preflight.
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := RequireTypedContextSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("typed context store: %w", err)
	}
	return db, nil
}

// RequireTypedContextSchema validates the shared v3 schema at a single
// consistent read snapshot, with no writable initialization. It cannot be
// called on a partially upgraded database as a repair or compatibility path.
func RequireTypedContextSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("typed context schema: missing database")
	}
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
		// All identifiers are host-reviewed compile-time literals.
		if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM "+expected.table).Scan(&version); err != nil {
			return fmt.Errorf("typed context schema %s: %w", expected.table, err)
		}
		if !version.Valid || version.Int64 != expected.value {
			return fmt.Errorf("typed context schema %s: expected version %d", expected.table, expected.value)
		}
	}
	for _, query := range []string{
		"SELECT source_kind,source_id FROM tools LIMIT 0",
		"SELECT profile_id,tool_id,workspace_id,sandbox_id FROM profile_tool_target_scopes LIMIT 0",
		"SELECT workspace_id,profile_id FROM workspace_profile_grants LIMIT 0",
		"SELECT sandbox_id,profile_id FROM sandbox_profile_grants LIMIT 0",
		"SELECT agent_id,profile_id FROM agent_grants LIMIT 0",
		"SELECT tool_id FROM native_tool_contracts LIMIT 0",
		"SELECT task_id,status FROM agent_tasks LIMIT 0",
		"SELECT approval_id,status FROM agent_task_approvals LIMIT 0",
		"SELECT task_id,agent_id FROM agent_native_sessions LIMIT 0",
		"SELECT sandbox_id,generation FROM sandbox_runs LIMIT 0",
		"SELECT sandbox_id,lease_token FROM sandbox_execution_leases LIMIT 0",
		"SELECT sandbox_id,enrolled_at FROM sandbox_ssh_job_obligations LIMIT 0",
		"SELECT workspace_id,sandbox_id FROM workspace_placements LIMIT 0",
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("typed context schema missing authority/evidence: %w", err)
		}
	}
	for _, old := range []string{
		"agent_tool_facades", "sandbox_tool_suite_bindings", "sandbox_job_suite_bindings", "workspace_tool_targets",
	} {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", old).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("typed context schema: legacy facade table still present")
		}
	}
	return tx.Commit()
}
