package configstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenMigratesV1ConfigSchemaToHeadersTemplateColumn(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE mcp_config_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE mcp_provider_configs (
		provider_id TEXT PRIMARY KEY,
		transport TEXT NOT NULL,
		endpoint TEXT NOT NULL,
		executable TEXT NOT NULL,
		args_json TEXT NOT NULL,
		directory TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO mcp_config_schema_migrations(version, applied_at) VALUES (1, 'v1')",
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
		"SELECT MAX(version) FROM mcp_config_schema_migrations",
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("schema version = %d, want 3", version)
	}

	rows, err := store.db.QueryContext(
		context.Background(),
		"PRAGMA table_info(mcp_provider_configs)",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	foundHeaders := false
	foundCredentialHeaders := false
	foundCredentialEnvironment := false
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
		switch name {
		case "headers_json":
			foundHeaders = true
		case "credential_headers_json":
			foundCredentialHeaders = true
		case "credential_environment_json":
			foundCredentialEnvironment = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !foundHeaders || !foundCredentialHeaders || !foundCredentialEnvironment {
		t.Fatalf(
			"expected migrated auth columns; headers=%v credentialHeaders=%v credentialEnvironment=%v",
			foundHeaders,
			foundCredentialHeaders,
			foundCredentialEnvironment,
		)
	}
}
