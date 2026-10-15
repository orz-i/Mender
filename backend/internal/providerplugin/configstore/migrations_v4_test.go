package configstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const legacyMCPID = "prov_00000000000000000000000000000031"

// prepareCanonicalV3 creates the previous canonical schema and its obsolete
// projection using separate raw SQLite handles. All tests use disposable files.
func prepareCanonicalV3(t *testing.T, canonical bool, pluginID string, configVersion int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range []string{
		"CREATE TABLE provider_config_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)",
		"INSERT INTO provider_config_schema_migrations VALUES(1,'v1'),(2,'v2'),(3,'v3')",
		"CREATE TABLE provider_configs(provider_id TEXT PRIMARY KEY,plugin_id TEXT NOT NULL,config_version INTEGER NOT NULL,revision TEXT NOT NULL,config_json TEXT NOT NULL,plugin_installation_id TEXT,credential_refs_json TEXT,connection_summary TEXT)",
		"CREATE TABLE provider_config_imports(name TEXT PRIMARY KEY,applied_at TEXT NOT NULL)",
		"INSERT INTO provider_config_imports VALUES('legacy-mcp-v3','old')",
		"CREATE TABLE mcp_config_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)",
		"INSERT INTO mcp_config_schema_migrations VALUES(4,'v4')",
		"CREATE TABLE mcp_provider_configs(provider_id TEXT PRIMARY KEY, transport TEXT)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO mcp_provider_configs VALUES(?,?)", legacyMCPID, "http"); err != nil {
		t.Fatal(err)
	}
	if canonical {
		if _, err := db.Exec("INSERT INTO provider_configs(provider_id,plugin_id,config_version,revision,config_json) VALUES(?,?,?,?,?)", legacyMCPID, pluginID, configVersion, "v3", `{"transport":{"kind":"streamable-http","endpoint":"https://example.test/mcp"}}`); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestProviderV4HardCutDropsOnlyRedundantMCPProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := prepareCanonicalV3(t, true, "io.mender.provider.mcp", 1)
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	var version int
	if err := store.db.QueryRowContext(ctx, "SELECT MAX(version) FROM provider_config_schema_migrations").Scan(&version); err != nil || version != 4 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	for _, table := range []string{"mcp_provider_configs", "mcp_config_schema_migrations", "provider_config_imports"} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("retired table survives: %s", table)
		}
	}
	var id string
	if err := store.db.QueryRowContext(ctx, "SELECT provider_id FROM provider_configs WHERE provider_id=?", legacyMCPID).Scan(&id); err != nil || id != legacyMCPID {
		t.Fatalf("canonical Provider lost or retargeted: %s, %v", id, err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO provider_configs(provider_id,plugin_id,config_version,revision,config_json)
    VALUES('prov_00000000000000000000000000000032','io.mender.provider.workspace',1,'v1','{"root":"/tmp"}')`); err == nil || !strings.Contains(err.Error(), "Workspace Provider v1 retired") {
		t.Fatalf("Workspace v1 reintroduced: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE provider_configs SET plugin_id='io.mender.provider.workspace',config_version=1 WHERE provider_id=?", legacyMCPID); err == nil || !strings.Contains(err.Error(), "Workspace Provider v1 retired") {
		t.Fatalf("Workspace v1 retargeted: %v", err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("idempotent v4 reopen: %v", err)
	}
	_ = reopened.Close()
}

func TestProviderV4HardCutRollsBackUnmappedMCP(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		canonical bool
		plugin    string
		version   int
	}{
		{"missing", false, "", 0}, {"wrong-plugin", true, "io.mender.provider.workspace", 2},
		{"wrong-version", true, "io.mender.provider.mcp", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := prepareCanonicalV3(t, tc.canonical, tc.plugin, tc.version)
			store, err := Open(context.Background(), path)
			if err == nil {
				_ = store.Close()
				t.Fatal("unmapped legacy MCP silently removed")
			}
			if !strings.Contains(err.Error(), "no valid canonical MCP Provider configuration") {
				t.Fatalf("unexpected failure %v", err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			var version, oldTable int
			if err := db.QueryRow("SELECT MAX(version) FROM provider_config_schema_migrations").Scan(&version); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='mcp_provider_configs'").Scan(&oldTable); err != nil {
				t.Fatal(err)
			}
			if version != 3 || oldTable != 1 {
				t.Fatalf("failed hard cut mutated source: v%d legacy=%d", version, oldTable)
			}
		})
	}
}

func TestProviderV4HardCutRejectsWorkspaceV1(t *testing.T) {
	t.Parallel()
	path := prepareCanonicalV3(t, true, "io.mender.provider.mcp", 1)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO provider_configs(provider_id,plugin_id,config_version,revision,config_json)
   VALUES('prov_00000000000000000000000000000033','io.mender.provider.workspace',1,'old','{"root":"/tmp"}')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), path)
	if err == nil {
		_ = store.Close()
		t.Fatal("Workspace v1 silently migrated")
	}
	if !strings.Contains(err.Error(), "not v2-only") {
		t.Fatalf("unexpected error: %v", err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM provider_config_schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("error advanced provider schema to %d", version)
	}
}

func TestNewProviderStoreNeverCreatesCompatibilityTables(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	for _, table := range []string{"mcp_provider_configs", "mcp_config_schema_migrations", "provider_config_imports"} {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("new Provider store contains retired table %s", table)
		}
	}
}
