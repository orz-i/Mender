package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	_ "modernc.org/sqlite"
)

func TestOpenMigratesV2IsolationToTunnelClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		"CREATE TABLE runtime_governance_schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)",
		"INSERT INTO runtime_governance_schema_migrations(version, applied_at) VALUES (1, 'v1'), (2, 'v2')",
		"CREATE TABLE runtime_isolation_settings (singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1), workspace_shell TEXT NOT NULL, external_provider TEXT NOT NULL, updated_at TEXT NOT NULL)",
		"INSERT INTO runtime_isolation_settings(singleton_id, workspace_shell, external_provider, updated_at) VALUES (1, 'host', 'host', 'v2')",
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

	store, err := Open(ctx, path, runtimegovernancedomain.DefaultIsolation(true))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	isolation, err := store.GetIsolation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if isolation.WorkspaceShell() != runtimegovernancedomain.IsolationHost ||
		isolation.ExternalProvider() != runtimegovernancedomain.IsolationHost ||
		isolation.TunnelClient() != runtimegovernancedomain.IsolationRestrictedHost {
		t.Fatalf("migrated isolation = %#v", isolation.Spec())
	}
}
