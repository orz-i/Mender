package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// v9 already has exact built-in extension pins but not external config / op
// journals. Upgrading v9 must make a consistent original backup and preserve
// all existing user Sandbox identities without probing a backend.
func TestSandboxV9ToV10ExternalSchemaUpgradePreservesInventory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID:   "sbx_11111111111111111111111111111111",
		Name: "Persisted local workspace", Backend: sandboxdomain.BackendLocalWorkspace,
		Binding:   sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace),
		Isolation: sandboxdomain.IsolationHost, LocalRoot: filepath.Join(t.TempDir(), "test-workspace"),
		OwnerKind: "local_user", OwnerRef: "owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(item.ID(), 7, sandboxdomain.StateReady, "local_workspace:"+item.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateReady(ctx, item, run); err == nil {
		t.Fatal("CreateReady accepted non-initial generation")
	}
	if err := store.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DROP TRIGGER sandbox_extension_binding_insert",
		"DROP TRIGGER sandbox_extension_binding_update",
		"DROP TRIGGER sandbox_ssh_template_immutable_update",
		"DROP TRIGGER sandbox_ssh_template_immutable_delete",
		"DROP TABLE sandbox_ssh_remote_templates",
		"DROP TABLE IF EXISTS sandbox_ssh_job_protocol_events", "DROP TABLE IF EXISTS sandbox_ssh_job_protocols", "DELETE FROM sandbox_schema_migrations WHERE version=18", "DELETE FROM sandbox_schema_migrations WHERE version=17",
		"DROP TABLE sandbox_ssh_job_recovery_events", "DELETE FROM sandbox_schema_migrations WHERE version=16",
		"DROP TABLE sandbox_ssh_job_admission_events",
		"DROP TABLE sandbox_ssh_job_admission_controls",
		"DELETE FROM sandbox_schema_migrations WHERE version=15",
		"DROP TABLE sandbox_job_suite_bindings",
		"DELETE FROM sandbox_schema_migrations WHERE version=14",
		"DROP TABLE sandbox_tool_suite_bindings",
		"DROP TABLE sandbox_ssh_job_obligations",
		"DELETE FROM sandbox_schema_migrations WHERE version=13",
		"DROP TRIGGER sandbox_external_config_update",
		"DROP TRIGGER sandbox_external_config_delete",
		"DROP TABLE sandbox_external_operations",
		"DROP TABLE sandbox_external_configs",
		"CREATE TRIGGER sandbox_extension_binding_insert BEFORE INSERT ON sandboxes BEGIN SELECT 1; END",
		"CREATE TRIGGER sandbox_extension_binding_update BEFORE UPDATE ON sandboxes BEGIN SELECT 1; END",
		"DELETE FROM sandbox_schema_migrations WHERE version=10",
		"DELETE FROM sandbox_schema_migrations WHERE version=11",
		"DELETE FROM sandbox_schema_migrations WHERE version=12",
	} {
		if _, err := raw.ExecContext(ctx, statement); err != nil {
			t.Fatalf("v9 fixture %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("v9 to v10 migration: %v", err)
	}
	retrieved, err := upgraded.Get(ctx, item.ID())
	if err != nil || retrieved.Spec() != item.Spec() {
		t.Fatalf("migrated Sandbox changed: %+v %v", retrieved.Spec(), err)
	}
	retained, err := upgraded.GetRun(ctx, item.ID())
	if err != nil || retained != run {
		t.Fatalf("migrated run changed %+v %v", retained, err)
	}
	var version int
	if err := upgraded.db.QueryRowContext(ctx, "SELECT MAX(version) FROM sandbox_schema_migrations").Scan(&version); err != nil || version != 18 {
		t.Fatalf("new schema version=%d %v", version, err)
	}
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	snapshots, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mender-sandbox-v9-backup-*.db"))
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("original v9 snapshot missing: %+v %v", snapshots, err)
	}
	backup, err := sql.Open("sqlite", snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = backup.Close() }()
	if err := backup.QueryRowContext(ctx, "SELECT MAX(version) FROM sandbox_schema_migrations").Scan(&version); err != nil || version != 9 {
		t.Fatalf("preupgrade snapshot schema=%d %v", version, err)
	}
	var count int
	if err := backup.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandboxes WHERE id=?", item.ID().String()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("preupgrade sandbox missing: %d %v", count, err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	after, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mender-sandbox-v9-backup-*.db"))
	if err != nil || len(after) != 1 {
		t.Fatalf("repeat migration caused extra backup: %+v %v", after, err)
	}
}
