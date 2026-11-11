package v3

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// Strict v3 schema admission is shared by every Context Store. A single stale
// ownership, audit or governance version must reject the entire Runtime,
// never repair one table and leave other domains partially initialized.
func TestPinnedV3RejectsEveryContextVersionMismatchWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		table    string
		mutation string
	}{
		{"tool_hardcut_schema_migrations", ""},
		{"catalog_schema_migrations", "UPDATE catalog_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM catalog_schema_migrations)"},
		{"profile_schema_migrations", "UPDATE profile_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM profile_schema_migrations)"},
		{"policy_schema_migrations", "UPDATE policy_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM policy_schema_migrations)"},
		{"agent_schema_migrations", "UPDATE agent_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM agent_schema_migrations)"},
		{"sandbox_schema_migrations", "UPDATE sandbox_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM sandbox_schema_migrations)"},
		{"provider_config_schema_migrations", "UPDATE provider_config_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM provider_config_schema_migrations)"},
		{"mcp_delivery_schema_migrations", "UPDATE mcp_delivery_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM mcp_delivery_schema_migrations)"},
		{"credential_schema_migrations", "UPDATE credential_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM credential_schema_migrations)"},
		{"skill_installation_schema_migrations", "UPDATE skill_installation_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM skill_installation_schema_migrations)"},
		{"tunnel_schema_migrations", "UPDATE tunnel_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM tunnel_schema_migrations)"},
		{"audit_schema_migrations", "UPDATE audit_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM audit_schema_migrations)"},
		{"plugin_installation_schema_migrations", "UPDATE plugin_installation_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM plugin_installation_schema_migrations)"},
		{"runtime_governance_schema_migrations", "UPDATE runtime_governance_schema_migrations SET version=999 WHERE version=(SELECT MAX(version) FROM runtime_governance_schema_migrations)"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "v3.sqlite")
			if err := EnsureFreshDatabase(path); err != nil {
				t.Fatal(err)
			}
			original, err := OpenContextStore(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if err = original.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.table == "tool_hardcut_schema_migrations" {
				// This marker is intrinsically constrained to version 1. A
				// future-version fixture must replace that constrained shape.
				if _, err = db.ExecContext(context.Background(), `DROP TABLE tool_hardcut_schema_migrations`); err != nil {
					t.Fatal(err)
				}
				if _, err = db.ExecContext(context.Background(), `CREATE TABLE tool_hardcut_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
					t.Fatal(err)
				}
				if _, err = db.ExecContext(context.Background(), `INSERT INTO tool_hardcut_schema_migrations VALUES(999,'future')`); err != nil {
					t.Fatal(err)
				}
			} else if _, err = db.ExecContext(context.Background(), tc.mutation); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			rejected, err := OpenContextStore(context.Background(), path)
			if rejected != nil {
				_ = rejected.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tc.table) {
				t.Fatalf("stale context accepted/incorrect diagnostic: %v", err)
			}
			inspect, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = inspect.Close() }()
			var current int
			if err = inspect.QueryRowContext(context.Background(), "SELECT MAX(version) FROM "+tc.table).Scan(&current); err != nil {
				t.Fatal(err)
			}
			if current != 999 {
				t.Fatal("strict v3 preflight modified foreign schema marker")
			}
		})
	}
}

func TestPinnedV3RejectsAnyRetiredFacadeOrPluginInventoryTable(t *testing.T) {
	for _, item := range []struct{ table, createSQL, insertSQL string }{
		{"agent_tool_facades", "CREATE TABLE agent_tool_facades(legacy_data TEXT)", "INSERT INTO agent_tool_facades VALUES('do-not-import')"},
		{"sandbox_tool_suite_bindings", "CREATE TABLE sandbox_tool_suite_bindings(legacy_data TEXT)", "INSERT INTO sandbox_tool_suite_bindings VALUES('do-not-import')"},
		{"sandbox_job_suite_bindings", "CREATE TABLE sandbox_job_suite_bindings(legacy_data TEXT)", "INSERT INTO sandbox_job_suite_bindings VALUES('do-not-import')"},
		{"workspace_tool_targets", "CREATE TABLE workspace_tool_targets(legacy_data TEXT)", "INSERT INTO workspace_tool_targets VALUES('do-not-import')"},
		{"provider_plugin_installations", "CREATE TABLE provider_plugin_installations(legacy_data TEXT)", "INSERT INTO provider_plugin_installations VALUES('do-not-import')"},
		{"provider_plugin_installation_schema_migrations", "CREATE TABLE provider_plugin_installation_schema_migrations(legacy_data TEXT)", "INSERT INTO provider_plugin_installation_schema_migrations VALUES('do-not-import')"},
	} {
		t.Run(item.table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "v3.sqlite")
			if err := EnsureFreshDatabase(path); err != nil {
				t.Fatal(err)
			}
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = raw.ExecContext(context.Background(), item.createSQL); err != nil {
				t.Fatal(err)
			}
			if _, err = raw.ExecContext(context.Background(), item.insertSQL); err != nil {
				t.Fatal(err)
			}
			if err = raw.Close(); err != nil {
				t.Fatal(err)
			}
			result, err := OpenContextStore(context.Background(), path)
			if result != nil {
				_ = result.Close()
			}
			if err == nil || !strings.Contains(err.Error(), "legacy facade table") {
				t.Fatalf("obsolete table accepted: %s: %v", item.table, err)
			}
			inspect, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = inspect.Close() }()
			var existing string
			if err = inspect.QueryRowContext(context.Background(), "SELECT legacy_data FROM "+item.table).Scan(&existing); err != nil || existing != "do-not-import" {
				t.Fatal("rejected obsolete row mutated")
			}
		})
	}
}

func TestPinnedV3InitializationNeverRewritesAnExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	if err := EnsureFreshDatabase(path); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- path is constructed exclusively from this test's t.TempDir; no external input.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = EnsureFreshDatabase(path); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- same test-owned temporary schema file, verified before/after initialization.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("v3 initializer rewrote an existing file")
	}
	db, err := OpenContextStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var count int
	if err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tools WHERE source_kind='native'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 9 {
		t.Fatalf("pinned v3 native Tool contracts=%d, want 9", count)
	}
}

func TestOpenContextStoreRejectsUNCPath(t *testing.T) {
	for _, unc := range []string{
		`\\server\share\test.db`,
		`//server/share/test.db`,
	} {
		_, err := OpenContextStore(context.Background(), unc)
		if err == nil || !strings.Contains(err.Error(), "UNC database path not supported") {
			t.Fatalf("expected UNC rejection for %q, got: %v", unc, err)
		}
	}
}
