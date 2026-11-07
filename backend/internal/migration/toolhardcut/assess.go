package toolhardcut

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"hash"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

var legacyVersions = map[string]int{
	"catalog": 2, "profile": 7, "policy": 3, "agent": 19, "sandbox": 19,
	"provider_config": 4, "mcp_delivery": 1,
}

var snapshotTables = []string{
	"providers", "tools", "provider_configs", "agent_tool_facades",
	"sandbox_tool_suite_bindings", "sandbox_job_suite_bindings", "workspace_tool_targets",
	"profiles", "profile_tools", "mcp_delivery_configs", "mcp_delivery_direct_tools",
	"policy_settings", "policy_rules", "agents", "agent_workspaces", "agent_workspace_grants",
	"agent_tool_placements", "agent_tasks", "agent_native_sessions", "agent_execution_attempts",
	"agent_task_continuations", "sandboxes", "sandbox_runs", "sandbox_profile_grants",
	"sandbox_execution_leases", "sandbox_ssh_job_obligations",
	"sandbox_ssh_job_recovery_events", "sandbox_ssh_job_admission_controls",
}

// AssessPath only opens a pre-existing SQLite database read-only. It never
// creates a DB, activates a Provider, connects to SSH, asks an Agent to run,
// opens the Keychain, or writes state. The returned JSON contains no secrets.
func AssessPath(ctx context.Context, path string) (Report, error) {
	if strings.TrimSpace(path) == "" {
		return Report{}, fmt.Errorf("assess tool hard cut: database path required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Report{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return Report{}, err
	}
	if !info.Mode().IsRegular() {
		return Report{}, fmt.Errorf("assess tool hard cut: regular database file required")
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	q := uri.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	return Assess(ctx, db)
}

// Assess takes one consistent read transaction. The caller owns the handle;
// no data is written even if a live Runtime is concurrently connected.
func Assess(ctx context.Context, db *sql.DB) (Report, error) {
	if db == nil {
		return Report{}, fmt.Errorf("assess tool hard cut: database required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = tx.Rollback() }()
	return assessTx(ctx, tx)
}

func assessTx(ctx context.Context, tx *sql.Tx) (Report, error) {
	result := newReport()
	if err := preflightSchema(ctx, tx, &result); err != nil {
		return Report{}, err
	}
	if result.Counts.UnresolvedBlockers != 0 || len(result.Conflicts) > 0 {
		if err := result.finalize(); err != nil {
			return Report{}, err
		}
		return result, nil
	}
	fp, err := semanticFingerprint(ctx, tx)
	if err != nil {
		return Report{}, err
	}
	result.Snapshot.DatabaseFingerprint = fp
	state, err := inventorySources(ctx, tx, &result)
	if err != nil {
		return Report{}, err
	}
	if err := assessBindings(ctx, tx, &result, state); err != nil {
		return Report{}, err
	}
	if err := assessTasksAndLeases(ctx, tx, &result); err != nil {
		return Report{}, err
	}
	if err := result.finalize(); err != nil {
		return Report{}, err
	}
	return result, nil
}

func preflightSchema(ctx context.Context, tx *sql.Tx, report *Report) error {
	names := make([]string, 0, len(legacyVersions))
	for name := range legacyVersions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		table := name + "_schema_migrations"
		exists, err := tableExists(ctx, tx, table)
		if err != nil {
			return err
		}
		if !exists {
			report.block(BlockSchema, []string{table}, "missing schema version table")
			continue
		}
		var version sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM "+table).Scan(&version); err != nil {
			return err
		}
		report.Snapshot.SchemaVersions[name] = int(version.Int64)
		if !version.Valid || int(version.Int64) != legacyVersions[name] {
			report.block(BlockSchema, []string{table}, fmt.Sprintf("schema version %d is not approved baseline %d", version.Int64, legacyVersions[name]))
		}
	}
	for _, name := range snapshotTables {
		exists, err := tableExists(ctx, tx, name)
		if err != nil {
			return err
		}
		if !exists {
			report.block(BlockSchema, []string{name}, "missing required legacy table")
		}
	}
	// FTS is a virtual table managed independently and is validated explicitly.
	for _, name := range []string{"tools_fts", "catalog_tool_revision"} {
		exists, err := tableExists(ctx, tx, name)
		if err != nil {
			return err
		}
		if !exists {
			report.block(BlockSchema, []string{name}, "missing Catalog search/revision table")
		}
	}
	if len(report.Conflicts) > 0 {
		return nil
	}
	var integrity string
	if err := tx.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		report.block(BlockSchema, nil, "SQLite quick_check failed")
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		report.block(BlockSchema, nil, "SQLite foreign_key_check failed")
	}
	return rows.Err()
}

func tableExists(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
 WHERE name=? AND type IN ('table','view')`, name).Scan(&n)
	return n == 1, err
}

func appendFingerprint(h hash.Hash, fields ...string) {
	for _, field := range fields {
		var header [8]byte
		binary.BigEndian.PutUint64(header[:], uint64(len(field)))
		_, _ = h.Write(header[:])
		_, _ = h.Write([]byte(field))
	}
}
