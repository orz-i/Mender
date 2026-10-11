package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	_ "modernc.org/sqlite"
)

const (
	legacyLocalID   = "sbx_00000000000000000000000000000001"
	legacyDockerID  = "sbx_00000000000000000000000000000002"
	legacyDaytonaID = "sbx_00000000000000000000000000000003"
)

// Recreate a byte-compatible v8 schema from the current forward migrations:
// only the three C2 columns and their triggers differ between v8 and v9.
func seedSandboxV8(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, statement := range []string{
		"DROP TRIGGER sandbox_extension_binding_insert",
		"DROP TRIGGER sandbox_extension_binding_update",
		"ALTER TABLE sandboxes DROP COLUMN backend_plugin_id",
		"ALTER TABLE sandboxes DROP COLUMN backend_extension_id",
		"ALTER TABLE sandboxes DROP COLUMN backend_installation_id",
		"DELETE FROM sandbox_schema_migrations WHERE version=9",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("prepare v8 fixture %q: %v", statement, err)
		}
	}
	now := time.Date(2026, 10, 9, 5, 6, 7, 0, time.UTC).Format(time.RFC3339Nano)
	items := []struct{ id, name, backend, isolation, localRoot string }{
		{legacyLocalID, "Legacy Local", "local_workspace", "host", "/tmp/mender-v8-workspace"},
		{legacyDockerID, "Legacy Docker", "docker", "container", ""},
		{legacyDaytonaID, "Legacy Daytona", "daytona", "remote", ""},
	}
	for _, item := range items {
		if _, err := db.ExecContext(ctx, `INSERT INTO sandboxes(id,display_name,backend,isolation,local_root,owner_kind,owner_ref,created_at) VALUES(?,?,?,?,?,?,?,?)`,
			item.id, item.name, item.backend, item.isolation, item.localRoot, "local_user", "os:123", now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO sandbox_runs(sandbox_id,generation,state,external_id,updated_at) VALUES(?,5,'ready',?,?)`,
			item.id, "verified-old-external-id", now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sandbox_docker_templates(sandbox_id,image_digest,user_id,group_id,cpu_milli,memory_mib,workspace_mib,pid_limit,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		legacyDockerID, "docker.io/library/alpine@sha256:"+strings.Repeat("a", 64), 1000, 1000, 1000, 512, 128, 64, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sandbox_daytona_templates(sandbox_id,credential_ref,image_digest,target,cpu,memory_gib,disk_gib,ttl_minutes,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		legacyDaytonaID, "credential://demo/key", "sha256:"+strings.Repeat("b", 64), "us", 1, 2, 5, 30, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sandbox_execution_leases(sandbox_id,lease_token,generation,principal_kind,principal_ref,scope_kind,scope_ref,acquired_at) VALUES(?,?,?,?,?,?,?,?)`,
		legacyDockerID, strings.Repeat("c", 32), 5, "local_user", "os:123", "catalog", "", now); err != nil {
		t.Fatal(err)
	}
	profileID := "prof_" + strings.Repeat("d", 32)
	if _, err := db.ExecContext(ctx, `INSERT INTO sandbox_profile_grants(sandbox_id,profile_id,capability,granted_at) VALUES(?,?,?,?)`,
		legacyLocalID, profileID, "file.read", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sandbox_profile_grant_events(sandbox_id,profile_id,capability,action,principal_kind,principal_ref,origin,occurred_at) VALUES(?,?,?,?,?,?,?,?)`,
		legacyLocalID, profileID, "file.read", "grant", "local_user", "os:123", "cli", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sandbox_lifecycle_events(sandbox_id,action,previous_state,next_state,generation,principal_kind,principal_ref,origin,scope_kind,scope_ref,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		legacyDockerID, "sandbox.start", "stopped", "ready", 5, "local_user", "os:123", "cli", "catalog", "", now); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSandboxV8BindingHardCutPreservesRunsLeasesTemplatesGrants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := seedSandboxV8(t)
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("forward migration v8 to v9: %v", err)
	}
	for _, tc := range []struct {
		id   string
		kind sandboxdomain.BackendKind
	}{
		{legacyLocalID, sandboxdomain.BackendLocalWorkspace},
		{legacyDockerID, sandboxdomain.BackendDocker},
		{legacyDaytonaID, sandboxdomain.BackendDaytona},
	} {
		id := sandboxdomain.ID(tc.id)
		item, err := store.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if item.ID() != id || item.Backend() != tc.kind || item.Spec().Binding != sandboxdomain.BuiltinBinding(tc.kind) || item.OwnerRef() != "os:123" {
			t.Fatalf("migrated Sandbox changed identity/authority: %+v", item.Spec())
		}
		run, err := store.GetRun(ctx, id)
		if err != nil || run.Generation != 5 || run.ExternalID != "verified-old-external-id" || run.State != sandboxdomain.StateReady {
			t.Fatalf("migrated Run changed: %+v %v", run, err)
		}
	}
	active, err := store.ActiveLeaseCount(ctx, sandboxdomain.ID(legacyDockerID))
	if err != nil || active != 1 {
		t.Fatalf("v8 lease was lost %d %v", active, err)
	}
	counts := map[string]int{
		"sandbox_docker_templates": 1, "sandbox_daytona_templates": 1,
		"sandbox_profile_grants": 1, "sandbox_profile_grant_events": 1,
		"sandbox_lifecycle_events": 1,
	}
	for table, want := range counts {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("v8 %s changed: count=%d err=%v", table, count, err)
		}
	}
	// A legacy direct SQL create may no longer sneak in default-empty binding.
	if _, err := store.db.ExecContext(ctx, `INSERT INTO sandboxes(id,display_name,backend,isolation,local_root,owner_kind,owner_ref,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		"sbx_00000000000000000000000000000004", "Missing Pin", "docker", "container", "", "local_user", "os:123", time.Now().UTC().Format(time.RFC3339Nano)); err == nil {
		t.Fatal("legacy INSERT bypassed exact extension identity")
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE sandboxes SET backend_plugin_id='dev.example.other' WHERE id=?", legacyDockerID); err == nil {
		t.Fatal("mutated immutable software pin via SQL")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	snapshots, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mender-sandbox-v8-backup-*.db"))
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("missing pre-upgrade snapshot: %+v, %v", snapshots, err)
	}
	backup, err := sql.Open("sqlite", snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	var oldKind string
	if err := backup.QueryRowContext(ctx, "SELECT backend FROM sandboxes WHERE id=?", legacyDockerID).Scan(&oldKind); err != nil || oldKind != "docker" {
		t.Fatalf("backup did not preserve source resource: %q, %v", oldKind, err)
	}
	_ = backup.Close()
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	snapshotsAfter, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mender-sandbox-v8-backup-*.db"))
	if err != nil || len(snapshotsAfter) != 1 {
		t.Fatal("migration replayed after restart")
	}
}

func TestSandboxBindingMigrationUnknownKindRollsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := seedSandboxV8(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE sandboxes SET backend='unrecognized_driver' WHERE id=?", legacyDockerID); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	store, err := Open(ctx, path)
	if err == nil {
		_ = store.Close()
		t.Fatal("unknown legacy backend silently migrated")
	}
	if !strings.Contains(err.Error(), "unknown backend plugin identity") {
		t.Fatalf("wrong failure: %v", err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var id string
	if err := raw.QueryRowContext(ctx, "SELECT id FROM sandboxes WHERE backend='unrecognized_driver'").Scan(&id); err != nil || id != legacyDockerID {
		t.Fatalf("failed migration removed/changed source record: %s %v", id, err)
	}
	var version int
	if err := raw.QueryRowContext(ctx, "SELECT MAX(version) FROM sandbox_schema_migrations").Scan(&version); err != nil || version != 8 {
		t.Fatalf("failed migration advanced version=%d: %v", version, err)
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".mender-sandbox-v8-backup-") {
			found = true
		}
	}
	if !found {
		t.Fatal("failed migration lacks preupgrade recovery snapshot")
	}
}
