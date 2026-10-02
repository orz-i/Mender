package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenMigratesAuditV1ThroughLatestSchema(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "audit.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE audit_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE invocation_audit (
		invocation_id TEXT PRIMARY KEY,
		tool_id TEXT NOT NULL,
		provider_id TEXT,
		caller TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL,
		policy_kind TEXT NOT NULL,
		policy_reason TEXT NOT NULL,
		approval TEXT NOT NULL,
		status TEXT NOT NULL,
		result_kind TEXT NOT NULL,
		error_class TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO audit_schema_migrations(version, applied_at) VALUES (1, 'v1')",
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	var version int
	if err := store.db.QueryRowContext(
		context.Background(),
		"SELECT MAX(version) FROM audit_schema_migrations",
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 5 {
		t.Fatalf("schema version = %d, want 5", version)
	}

	rows, err := store.db.QueryContext(
		context.Background(),
		"PRAGMA table_info(invocation_audit)",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	found := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(
			&cid,
			&name,
			&columnType,
			&notNull,
			&defaultValue,
			&primaryKey,
		); err != nil {
			t.Fatal(err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"principal_kind",
		"principal_ref",
		"error_summary",
		"origin",
		"scope_kind",
		"scope_reference",
		"outcome_kind",
		"rejection_code",
		"failure_source",
		"failure_code",
	} {
		if !found[name] {
			t.Fatalf("%s column was not added", name)
		}
	}
	if found["caller"] {
		t.Fatal("caller column survived Principal hard cut")
	}
}
