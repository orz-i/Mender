package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// Actual v17 databases contain the immutable per-Sandbox protocol selector.
// Hard-cut v18 removes both the selector and its audit table, while keeping
// Sandbox Runs, owned Jobs obligations, pause controls, Policy and credentials.
func TestSSHJobsV17ToV18DropsSelectorWithoutResourceLoss(t *testing.T) {
	ctx := context.Background()
	filename, id, run, owner := newFencedSSHFixture(t)
	raw, err := sql.Open("sqlite", filename)
	if err != nil {
		t.Fatal(err)
	}
	rewindWorkspaceTargetsV19Fixture(t, raw)
	stmts := []string{
		"DELETE FROM sandbox_schema_migrations WHERE version=18",
		"CREATE TABLE sandbox_ssh_job_protocols(sandbox_id TEXT PRIMARY KEY,protocol INTEGER NOT NULL,generation INTEGER NOT NULL,activated_at TEXT NOT NULL)",
		"CREATE TABLE sandbox_ssh_job_protocol_events(event_id INTEGER PRIMARY KEY,sandbox_id TEXT NOT NULL,action TEXT NOT NULL)",
		"INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(17,'2026-10-14T00:00:00Z')",
		"INSERT INTO sandbox_ssh_job_protocols(sandbox_id,protocol,generation,activated_at) VALUES('" + id.String() + "',2,1,'2026-10-14T00:00:00Z')",
	}
	for _, stmt := range stmts {
		if _, err = raw.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	var schema int
	if err = store.db.QueryRowContext(ctx, "SELECT MAX(version) FROM sandbox_schema_migrations").Scan(&schema); err != nil || schema != 19 {
		t.Fatalf("hard cut schema: %d %v", schema, err)
	}
	for _, table := range []string{"sandbox_ssh_job_protocols", "sandbox_ssh_job_protocol_events"} {
		var exists int
		if err = store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&exists); err != nil || exists != 0 {
			t.Fatalf("selector survived v18: %s count=%d %v", table, exists, err)
		}
	}
	observed, err := store.GetRun(ctx, id)
	if err != nil || observed != run {
		t.Fatalf("resource Run lost: %+v %v", observed, err)
	}
	if err = store.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, true, owner); err != nil {
		t.Fatal(err)
	}
	state, err := store.SSHJobSubmissionsPaused(ctx, id)
	if err != nil || !state {
		t.Fatalf("pause admission lost after retirement: %v %v", state, err)
	}
}
