package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/providerplugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
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

func TestOpenFailsClosedOnFutureSchemaVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE provider_plugin_installation_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(
		ctx,
		"INSERT INTO provider_plugin_installation_schema_migrations(version, applied_at) VALUES (?, ?)",
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
	pluginID, err := providerplugin.ParseID(rawPluginID)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON := []byte(`{
		"manifestVersion": 1,
		"pluginId": "` + pluginID.String() + `",
		"version": "` + version + `",
		"protocolVersions": [1],
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
