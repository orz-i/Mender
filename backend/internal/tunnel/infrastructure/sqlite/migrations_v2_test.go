package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	_ "modernc.org/sqlite"
)

func TestOpenMigratesV1TunnelWithoutExecutable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		"CREATE TABLE tunnel_schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)",
		"INSERT INTO tunnel_schema_migrations(version, applied_at) VALUES (1, 'v1')",
		"CREATE TABLE tunnels (id TEXT PRIMARY KEY, display_name TEXT NOT NULL, provider TEXT NOT NULL, external_tunnel_id TEXT NOT NULL, runtime_credential_ref TEXT NOT NULL, executable TEXT NOT NULL, UNIQUE(provider, external_tunnel_id))",
		"INSERT INTO tunnels(id, display_name, provider, external_tunnel_id, runtime_credential_ref, executable) VALUES ('tun_00000000000000000000000000000001', 'OpenAI Work', 'openai', 'tunnel_external_1', 'credential://openai/tunnel-runtime', '/tmp/custom-tunnel-client')",
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	id, _ := tunneldomain.ParseTunnelID("tun_00000000000000000000000000000001")
	loaded, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := credentialdomain.ParseReference("credential://openai/tunnel-runtime")
	if loaded.DisplayName() != "OpenAI Work" ||
		loaded.Provider() != tunneldomain.ProviderOpenAI ||
		loaded.ExternalTunnelID() != "tunnel_external_1" ||
		loaded.RuntimeCredentialRef() != ref {
		t.Fatalf("migrated tunnel = %#v", loaded)
	}

	rows, err := store.db.QueryContext(ctx, "PRAGMA table_info(tunnels)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "executable" {
			t.Fatal("migration retained removed executable column")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
