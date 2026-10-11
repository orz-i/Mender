package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	_ "modernc.org/sqlite"
)

func TestStoreCreateGetListSetStatusDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t)
	first := testInstallation(
		t,
		"pinst_00000000000000000000000000000002",
		"dev.example.echo",
		"1.2.3",
		"a",
	)
	second := testInstallation(
		t,
		"pinst_00000000000000000000000000000001",
		"dev.example.other",
		"2.0.0",
		"b",
	)

	if err := store.Create(ctx, first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	if err := store.Create(ctx, second); err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	got, err := store.Get(ctx, first.ID())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertInstallationEqual(t, got, first)

	byDigest, err := store.GetByDigest(ctx, second.Digest())
	if err != nil {
		t.Fatalf("GetByDigest() error = %v", err)
	}
	assertInstallationEqual(t, byDigest, second)

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 ||
		items[0].ID() != second.ID() ||
		items[1].ID() != first.ID() {
		t.Fatalf("List() = %#v", items)
	}

	if err := store.SetStatus(ctx, first.ID(), installationdomain.StatusDisabled); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	disabled, err := store.Get(ctx, first.ID())
	if err != nil {
		t.Fatalf("Get(disabled) error = %v", err)
	}
	if disabled.Status() != installationdomain.StatusDisabled {
		t.Fatalf("Status() = %q", disabled.Status())
	}
	if disabled.Digest() != first.Digest() ||
		disabled.PluginID() != first.PluginID() ||
		disabled.PackageVersion() != first.PackageVersion() {
		t.Fatal("SetStatus() changed immutable installation metadata")
	}

	if err := store.Delete(ctx, first.ID()); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(ctx, first.ID()); !errors.Is(err, installationapplication.ErrInstallationNotFound) {
		t.Fatalf("Get(deleted) error = %v, want installationapplication.ErrInstallationNotFound", err)
	}
	if err := store.Delete(ctx, first.ID()); !errors.Is(err, installationapplication.ErrInstallationNotFound) {
		t.Fatalf("Delete(missing) error = %v, want installationapplication.ErrInstallationNotFound", err)
	}
}

func TestStorePersistsVerifiedDescriptorSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t)
	installation := testInstallation(
		t,
		"pinst_00000000000000000000000000000003",
		"dev.example.echo",
		"1.2.3",
		"c",
	)
	if err := store.Create(ctx, installation); err != nil {
		t.Fatal(err)
	}

	descriptor := platformplugin.Descriptor{
		PluginID:       installation.PluginID(),
		PackageVersion: installation.PackageVersion(),
		Extension:      installation.Extension(),
		DisplayName:    "Echo",
		Description:    "Verified descriptor",
		ConfigVersion:  2,
		ConfigSchema: []byte(`{
  "type": "object",
  "properties": {"prefix": {"type": "string"}}
}`),
		UISchema:     []byte(`{"properties":{"prefix":{"control":"text"}}}`),
		Capabilities: []string{"streaming", "credentials"},
	}
	if err := store.SetVerifiedDescriptor(ctx, installation.ID(), descriptor); err != nil {
		t.Fatalf("SetVerifiedDescriptor() error = %v", err)
	}
	if err := store.SetVerifiedDescriptor(ctx, installation.ID(), descriptor); err != nil {
		t.Fatalf("SetVerifiedDescriptor(idempotent) error = %v", err)
	}

	got, err := store.Get(ctx, installation.ID())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := got.VerifiedDescriptor()
	if !ok {
		t.Fatal("VerifiedDescriptor() missing after persistence")
	}
	if snapshot.PluginID != descriptor.PluginID ||
		snapshot.PackageVersion != descriptor.PackageVersion ||
		snapshot.Extension != descriptor.Extension ||
		snapshot.ConfigVersion != descriptor.ConfigVersion ||
		snapshot.DisplayName != descriptor.DisplayName ||
		!equivalentJSON(snapshot.ConfigSchema, descriptor.ConfigSchema) ||
		!equivalentJSON(snapshot.UISchema, descriptor.UISchema) {
		t.Fatalf("verified descriptor = %+v, want %+v", snapshot, descriptor)
	}

	changed := descriptor.Clone()
	changed.DisplayName = "Changed"
	if err := store.SetVerifiedDescriptor(ctx, installation.ID(), changed); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("SetVerifiedDescriptor(changed) error = %v", err)
	}
}

func TestStoreRejectsDuplicateIdentityAndDigest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t)
	first := testInstallation(
		t,
		"pinst_00000000000000000000000000000001",
		"dev.example.echo",
		"1.0.0",
		"a",
	)
	if err := store.Create(ctx, first); err != nil {
		t.Fatal(err)
	}

	sameID := testInstallation(
		t,
		first.ID().String(),
		"dev.example.other",
		"2.0.0",
		"b",
	)
	if err := store.Create(ctx, sameID); !errors.Is(err, installationapplication.ErrInstallationConflict) {
		t.Fatalf("Create(same ID) error = %v, want installationapplication.ErrInstallationConflict", err)
	}

	otherID := testInstallation(
		t,
		"pinst_00000000000000000000000000000002",
		"dev.example.echo",
		"1.0.0",
		"a",
	)
	if err := store.Create(ctx, otherID); !errors.Is(err, installationapplication.ErrInstallationConflict) {
		t.Fatalf("Create(same digest) error = %v, want installationapplication.ErrInstallationConflict", err)
	}
}

func TestStoreMissingAndStatusValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t)
	id, _ := installationdomain.ParseInstallationID(
		"pinst_00000000000000000000000000000001",
	)

	if _, err := store.Get(ctx, id); !errors.Is(err, installationapplication.ErrInstallationNotFound) {
		t.Fatalf("Get(missing) error = %v", err)
	}
	if _, err := store.GetByDigest(ctx, "sha256:"+strings.Repeat("f", 64)); !errors.Is(err, installationapplication.ErrInstallationNotFound) {
		t.Fatalf("GetByDigest(missing) error = %v", err)
	}
	if err := store.SetStatus(ctx, id, installationdomain.Status("broken")); err == nil {
		t.Fatal("SetStatus() accepted invalid status")
	}
	if err := store.SetStatus(ctx, id, installationdomain.StatusEnabled); !errors.Is(err, installationapplication.ErrInstallationNotFound) {
		t.Fatalf("SetStatus(missing) error = %v", err)
	}
}

func TestOpenHardCutsLegacyInstallationSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE provider_plugin_installation_schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE provider_plugin_installations (
			installation_id TEXT PRIMARY KEY,
			plugin_id TEXT NOT NULL,
			package_version TEXT NOT NULL,
			source_kind TEXT NOT NULL,
			source_reference TEXT NOT NULL,
			digest TEXT NOT NULL UNIQUE,
			manifest_json TEXT NOT NULL,
			status TEXT NOT NULL,
			installed_at TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(
		ctx,
		"INSERT INTO provider_plugin_installation_schema_migrations(version, applied_at) VALUES (1, ?)",
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("Open(v1) error = %v", err)
	}
	defer func() { _ = store.Close() }()
	rows, err := store.db.QueryContext(ctx, "PRAGMA table_info(plugin_installations)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	found := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "verified_descriptor_json" {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("generic plugin inventory is missing verified descriptor column")
	}
	var legacyTables int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='provider_plugin_installations'").Scan(&legacyTables); err != nil {
		t.Fatal(err)
	}
	if legacyTables != 0 {
		t.Fatal("old provider installation inventory still exists")
	}
	snapshots, err := filepath.Glob(filepath.Join(filepath.Dir(databasePath), ".mender-phase27-backup-*.db"))
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("preupgrade backup missing: %v %v", snapshots, err)
	}
}

func TestOpenFailsClosedOnFutureSchemaVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE plugin_installation_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(
		ctx,
		"INSERT INTO plugin_installation_schema_migrations(version, applied_at) VALUES (?, ?)",
		latestSchemaVersion+1,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, databasePath)
	if err == nil {
		_ = store.Close()
		t.Fatal("Open() error = nil for future schema")
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("Open() error = %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}

func testInstallation(
	t *testing.T,
	rawID string,
	rawPluginID string,
	version string,
	digestRune string,
) installationdomain.Installation {
	t.Helper()

	id, err := installationdomain.ParseInstallationID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := platformplugin.ParseID(rawPluginID)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON := []byte(`{
		"manifestVersion": 2,
		"pluginId": "` + pluginID.String() + `",
		"version": "` + version + `",
		"extensions": [{"id":"main","kind":"provider.tools","protocolVersion":1}],
		"displayName": "Test Plugin",
		"description": "Test external Provider plugin",
		"entrypoints": {"linux/amd64": "bin/plugin"}
	}`)
	item, err := installationdomain.NewInstallation(
		id,
		pluginID,
		version,
		installationdomain.SourceLocal,
		"/tmp/"+id.String(),
		"sha256:"+strings.Repeat(digestRune, 64),
		manifestJSON,
		installationdomain.StatusEnabled,
		time.Date(2026, 9, 27, 12, 0, 0, 123, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func assertInstallationEqual(
	t *testing.T,
	got installationdomain.Installation,
	want installationdomain.Installation,
) {
	t.Helper()
	if got.ID() != want.ID() ||
		got.PluginID() != want.PluginID() ||
		got.PackageVersion() != want.PackageVersion() ||
		got.SourceKind() != want.SourceKind() ||
		got.SourceReference() != want.SourceReference() ||
		got.Digest() != want.Digest() ||
		string(got.ManifestJSON()) != string(want.ManifestJSON()) ||
		got.Status() != want.Status() ||
		!got.InstalledAt().Equal(want.InstalledAt()) {
		t.Fatalf("installation mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func equivalentJSON(a, b []byte) bool {
	var x, y bytes.Buffer
	if err := json.Compact(&x, a); err != nil {
		return false
	}
	if err := json.Compact(&y, b); err != nil {
		return false
	}
	return bytes.Equal(x.Bytes(), y.Bytes())
}
