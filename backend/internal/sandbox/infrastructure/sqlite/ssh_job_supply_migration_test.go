package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// Stage C v14 never overwrites Workspace grants or audit/event IDs and never
// introduces a Core task store. A standalone Sandbox DB may lack providers.
func TestSandboxV13ToV14JobsSupplyPreservesWorkspaceGrants(t *testing.T) {
	filename, id, run, _ := newFencedSSHFixture(t)
	ctx := context.Background()
	raw, err := sql.Open("sqlite", filename)
	if err != nil {
		t.Fatal(err)
	}
	rewindWorkspaceTargetsV19Fixture(t, raw)
	profile := "prof_0123456789abcdef0123456789abcdef"
	_, err = raw.ExecContext(ctx, `INSERT INTO sandbox_profile_grants(sandbox_id,profile_id,capability,granted_at) VALUES(?,?,'file.read','2026-10-13T00:00:00Z')`, id.String(), profile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.ExecContext(ctx, `INSERT INTO sandbox_profile_grant_events(event_id,sandbox_id,profile_id,capability,action,principal_kind,principal_ref,origin,occurred_at) VALUES(4321,?,?,'file.read','grant','local_user','os:fixture','cli','2026-10-13T00:00:00Z')`, id.String(), profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"DROP TABLE IF EXISTS sandbox_ssh_job_protocol_events", "DROP TABLE IF EXISTS sandbox_ssh_job_protocols", "DELETE FROM sandbox_schema_migrations WHERE version=18", "DELETE FROM sandbox_schema_migrations WHERE version=17",
		"DROP TABLE sandbox_ssh_job_recovery_events", "DELETE FROM sandbox_schema_migrations WHERE version=16",
		"DROP TABLE sandbox_ssh_job_admission_events", "DROP TABLE sandbox_ssh_job_admission_controls", "DELETE FROM sandbox_schema_migrations WHERE version=15"} {
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
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = upgraded.Close() }()
	got, err := upgraded.GetRun(ctx, id)
	if err != nil || got != run {
		t.Fatalf("run identity changed %+v %v", got, err)
	}
	allowed, err := upgraded.HasProfileGrant(ctx, id, profile, "file.read")
	if err != nil || !allowed {
		t.Fatalf("Workspace grant lost: %v %v", allowed, err)
	}
	var eventID int
	if err = upgraded.db.QueryRowContext(ctx, `SELECT event_id FROM sandbox_profile_grant_events WHERE sandbox_id=?`, id.String()).Scan(&eventID); err != nil || eventID != 4321 {
		t.Fatalf("audit event ID changed %d %v", eventID, err)
	}
	var version int
	if err = upgraded.db.QueryRowContext(ctx, `SELECT MAX(version) FROM sandbox_schema_migrations`).Scan(&version); err != nil || version != 19 {
		t.Fatalf("v14=%d %v", version, err)
	}
	var jobsCount int
	if err = upgraded.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_job_suite_bindings`).Scan(&jobsCount); err != nil || jobsCount != 0 {
		t.Fatalf("implicit Jobs suite %d %v", jobsCount, err)
	}
	var taskTables int
	if err = upgraded.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE '%task%'`).Scan(&taskTables); err != nil || taskTables != 0 {
		t.Fatalf("Core became remote task store %d %v", taskTables, err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".mender-sandbox-v10-backup-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("pre-v14 backup count %d %v", len(backups), err)
	}
	backup, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = backup.Close() }()
	if err = backup.QueryRowContext(ctx, `SELECT MAX(version) FROM sandbox_schema_migrations`).Scan(&version); err != nil || version != 13 {
		t.Fatalf("pre-v14 snapshot version %d %v", version, err)
	}
}
