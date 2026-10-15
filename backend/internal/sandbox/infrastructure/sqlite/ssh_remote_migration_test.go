package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	_ "modernc.org/sqlite"
)

// A v10 user database is snapshotted before forward-only SSH schema v11.
// Existing IDs, Runs and filesystem roots must survive without any remote I/O.
func TestSSHRemoteV10ToV11MigrationPreservesInventoryAndCreatesSingleBackup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	filename := filepath.Join(t.TempDir(), "mender.db")
	store, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	id := sandboxdomain.ID("sbx_0000000000000000000000000000dabc")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID: id, Name: "Preexisting local resource", Backend: sandboxdomain.BackendLocalWorkspace,
		Binding:   sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace),
		Isolation: sandboxdomain.IsolationHost, LocalRoot: filepath.Join(t.TempDir(), "mender-preexisting-root"),
		OwnerKind: "local_user", OwnerRef: "os:existing",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "local_workspace:"+id.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateReady(ctx, item, run); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", filename)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
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
		"DELETE FROM sandbox_schema_migrations WHERE version=12",
		"DELETE FROM sandbox_schema_migrations WHERE version=11",
	} {
		if _, err := raw.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	got, err := upgraded.Get(ctx, id)
	if err != nil || got.Spec() != item.Spec() {
		t.Fatalf("migrated item %+v %v", got.Spec(), err)
	}
	observed, err := upgraded.GetRun(ctx, id)
	if err != nil || observed != run {
		t.Fatalf("migrated run %+v %v", observed, err)
	}
	var version int
	if err := upgraded.db.QueryRowContext(ctx, "SELECT MAX(version) FROM sandbox_schema_migrations").Scan(&version); err != nil || version != 18 {
		t.Fatalf("new schema version=%d %v", version, err)
	}
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	snap, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".mender-sandbox-v10-backup-*.db"))
	if err != nil || len(snap) != 1 {
		t.Fatalf("missing consistent v10 snapshot: %+v %v", snap, err)
	}
	backup, err := sql.Open("sqlite", snap[0])
	if err != nil {
		t.Fatal(err)
	}
	var original string
	if err := backup.QueryRowContext(ctx, "SELECT display_name FROM sandboxes WHERE id=?", id.String()).Scan(&original); err != nil || original != item.Spec().Name {
		t.Fatalf("pre-upgrade backup did not preserve item: %q %v", original, err)
	}
	var previous int
	if err := backup.QueryRowContext(ctx, "SELECT MAX(version) FROM sandbox_schema_migrations").Scan(&previous); err != nil || previous != 10 {
		t.Fatalf("pre-upgrade backup version=%d %v", previous, err)
	}
	if err := backup.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	_ = reopened.Close()
	remaining, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".mender-sandbox-v10-backup-*.db"))
	if err != nil || len(remaining) != 1 {
		t.Fatalf("migration created duplicate snapshots: %+v %v", remaining, err)
	}
}
