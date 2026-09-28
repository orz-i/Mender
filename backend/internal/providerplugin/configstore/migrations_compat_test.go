package configstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	_ "modernc.org/sqlite"
)

func TestOpenMigratesV1ProviderConfigWithNullInstallationPin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE provider_config_schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE provider_configs (
			provider_id TEXT PRIMARY KEY,
			plugin_id TEXT NOT NULL,
			config_version INTEGER NOT NULL,
			revision TEXT NOT NULL,
			config_json TEXT NOT NULL
		)`,
		`CREATE TABLE provider_config_imports (
			name TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		"INSERT INTO provider_config_schema_migrations(version, applied_at) VALUES (1, 'legacy')",
		`INSERT INTO provider_configs(
			provider_id, plugin_id, config_version, revision, config_json
		) VALUES (
			'prov_00000000000000000000000000000031',
			'io.mender.provider.mcp',
			1,
			'sha256:legacy',
			'{"transport":{"kind":"stdio"}}'
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	providerID, err := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000031",
	)
	if err != nil {
		t.Fatal(err)
	}
	config, err := store.GetProviderConfig(context.Background(), providerID)
	if err != nil {
		t.Fatal(err)
	}
	if config.InstallationID != nil {
		t.Fatalf("InstallationID = %v, want nil for migrated built-in", config.InstallationID)
	}

	var version int
	if err := store.db.QueryRow(
		"SELECT MAX(version) FROM provider_config_schema_migrations",
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != latestSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, latestSchemaVersion)
	}
}

func TestOpenRejectsFutureProviderConfigSchemaVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE provider_config_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO provider_config_schema_migrations(version, applied_at) VALUES (99, 'future')",
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "newer than supported version") {
		t.Fatalf("Open() error = %v, want future schema rejection", err)
	}
}
