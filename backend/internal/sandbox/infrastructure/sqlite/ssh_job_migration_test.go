package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// The v13 migration creates only an SSH resource obligation. It must not
// create a RemoteTask table, change existing Sandbox IDs/Runs/leases or
// trigger a live SSH probe. Existing v12 databases receive a consistent
// pre-upgrade snapshot once, not on every Runtime start.
func TestSandboxV12ToV13SSHJobObligationMigration(t *testing.T) {
	filename, id, run, _ := newFencedSSHFixture(t)
	ctx := context.Background()
	raw, err := sql.Open("sqlite", filename)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"DROP TABLE sandbox_ssh_job_admission_events", "DROP TABLE sandbox_ssh_job_admission_controls", "DELETE FROM sandbox_schema_migrations WHERE version=15"} {
		if _, e := raw.ExecContext(ctx, statement); e != nil {
			t.Fatal(e)
		}
	}
	if _, err = raw.ExecContext(ctx, `DROP TABLE sandbox_job_suite_bindings`); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.ExecContext(ctx, `DELETE FROM sandbox_schema_migrations WHERE version=14`); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.ExecContext(ctx, `DROP TABLE sandbox_ssh_job_obligations`); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.ExecContext(ctx, `DELETE FROM sandbox_schema_migrations WHERE version=13`); err != nil {
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := upgraded.GetRun(ctx, id)
	if err != nil || observed != run {
		t.Fatalf("v12 inventory changed %+v %v", observed, err)
	}
	managed, err := upgraded.SSHJobManaged(ctx, id)
	if err != nil || managed {
		t.Fatalf("migration implicitly enrolled job: %t %v", managed, err)
	}
	var version int
	if err = upgraded.db.QueryRowContext(ctx, `SELECT MAX(version) FROM sandbox_schema_migrations`).Scan(&version); err != nil || version != 15 {
		t.Fatalf("upgrade version: %d %v", version, err)
	}
	var taskTables int
	if err = upgraded.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE '%task%'`).Scan(&taskTables); err != nil || taskTables != 0 {
		t.Fatalf("migration introduced a task control plane: %d %v", taskTables, err)
	}
	if err = upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".mender-sandbox-v10-backup-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("v12 snapshot count %d %v", len(backups), err)
	}
	backup, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = backup.QueryRowContext(ctx, `SELECT MAX(version) FROM sandbox_schema_migrations`).Scan(&version); err != nil || version != 12 {
		t.Fatalf("preupgrade snapshot version %d %v", version, err)
	}
	_ = backup.Close()
	reopened, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	_ = reopened.Close()
	after, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".mender-sandbox-v10-backup-*.db"))
	if err != nil || len(after) != 1 {
		t.Fatalf("repeat upgrade duplicated snapshot: %d %v", len(after), err)
	}
}
