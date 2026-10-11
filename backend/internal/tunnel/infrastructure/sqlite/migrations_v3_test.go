package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	_ "modernc.org/sqlite"
)

func TestTunnelV2ToV3HardCutPreservesProfileAndCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"CREATE TABLE tunnel_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)",
		"INSERT INTO tunnel_schema_migrations VALUES(1,'v1'),(2,'v2')",
		"CREATE TABLE tunnels(id TEXT PRIMARY KEY,display_name TEXT NOT NULL,provider TEXT NOT NULL,external_tunnel_id TEXT NOT NULL,runtime_credential_ref TEXT NOT NULL,UNIQUE(provider,external_tunnel_id))",
		"INSERT INTO tunnels VALUES('tun_00000000000000000000000000000001','Historical OpenAI','openai','tunnel_old','credential://openai/key')",
		"CREATE TABLE profiles(id TEXT PRIMARY KEY,slug TEXT NOT NULL,display_name TEXT NOT NULL,exposure_mode TEXT NOT NULL,tunnel_id TEXT NOT NULL DEFAULT '')",
		"INSERT INTO profiles VALUES('prof_11111111111111111111111111111111','existing','Existing','direct','tun_00000000000000000000000000000001')",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := tunneldomain.ParseTunnelID("tun_00000000000000000000000000000001")
	item, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := credentialdomain.ParseReference("credential://openai/key")
	if item.ID() != id || item.Provider() != tunneldomain.ProviderOpenAI ||
		item.ExternalTunnelID() != "tunnel_old" || item.RuntimeCredentialRef() != ref ||
		item.Binding() != tunneldomain.OpenAIBinding() {
		t.Fatalf("migrated tunnel changed: %+v", item)
	}
	var linked string
	if err := store.db.QueryRowContext(ctx, "SELECT tunnel_id FROM profiles WHERE slug='existing'").Scan(&linked); err != nil || linked != id.String() {
		t.Fatalf("Profile->Tunnel reference changed: %q %v", linked, err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE tunnels SET plugin_id='dev.example.attacker' WHERE id=?", id); err == nil {
		t.Fatal("immutable extension identity changed through SQL")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	snapshots, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mender-tunnel-pre-v3-*.db"))
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("pre-upgrade backup missing %+v %v", snapshots, err)
	}
	snapshot, err := sql.Open("sqlite", snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = snapshot.Close() }()
	var oldProvider string
	if err := snapshot.QueryRowContext(ctx, "SELECT provider FROM tunnels WHERE id=?", id).Scan(&oldProvider); err != nil || oldProvider != "openai" {
		t.Fatalf("backup did not preserve old resource: %s %v", oldProvider, err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	again, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mender-tunnel-pre-v3-*.db"))
	if err != nil || len(again) != 1 {
		t.Fatalf("migration replayed backup %+v %v", again, err)
	}
}

func TestTunnelV2UnknownProviderRollback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"CREATE TABLE tunnel_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)",
		"INSERT INTO tunnel_schema_migrations VALUES(1,'v1'),(2,'v2')",
		"CREATE TABLE tunnels(id TEXT PRIMARY KEY,display_name TEXT NOT NULL,provider TEXT NOT NULL,external_tunnel_id TEXT NOT NULL,runtime_credential_ref TEXT NOT NULL,UNIQUE(provider,external_tunnel_id))",
		"INSERT INTO tunnels VALUES('tun_00000000000000000000000000000001','Unknown','untrusted','old','credential://test/key')",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err == nil {
		_ = store.Close()
		t.Fatal("unknown legacy provider migrated")
	}
	if !strings.Contains(err.Error(), "unsupported legacy tunnel providers") {
		t.Fatalf("unexpected migration failure: %v", err)
	}
	verify, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = verify.Close() }()
	var got string
	if err := verify.QueryRowContext(ctx, "SELECT provider FROM tunnels").Scan(&got); err != nil || got != "untrusted" {
		t.Fatalf("failed migration modified original table: %q %v", got, err)
	}
	var version int
	if err := verify.QueryRowContext(ctx, "SELECT MAX(version) FROM tunnel_schema_migrations").Scan(&version); err != nil || version != 2 {
		t.Fatalf("failed migration advanced schema: %d %v", version, err)
	}
}
