package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	_ "modernc.org/sqlite"
)

func TestProviderPackageHardCutPreservesExactIdentityAndSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC)
	_, err = db.ExecContext(ctx, `CREATE TABLE provider_plugin_installation_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL);
CREATE TABLE provider_plugin_installations(
installation_id TEXT PRIMARY KEY,plugin_id TEXT NOT NULL,package_version TEXT NOT NULL,source_kind TEXT NOT NULL,
source_reference TEXT NOT NULL,digest TEXT NOT NULL UNIQUE,manifest_json TEXT NOT NULL,
verified_descriptor_json TEXT,status TEXT NOT NULL,installed_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO provider_plugin_installation_schema_migrations(version,applied_at) VALUES(2,?)", now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	const oldManifest = `{"manifestVersion":1,"pluginId":"dev.example.echo","version":"1.2.3","protocolVersions":[1],"displayName":"Legacy Echo","description":"test","entrypoints":{"darwin/arm64":"bin/echo"}}`
	const oldDescriptor = `{"pluginId":"dev.example.echo","packageVersion":"1.2.3","protocolVersion":1,"configVersion":2,"displayName":"Legacy Echo","description":"test","configSchema":"{\"type\":\"object\"}","capabilities":["echo"]}`
	const id = "pinst_0123456789abcdef0123456789abcdef"
	digest := "sha256:" + strings.Repeat("a", 64)
	_, err = db.ExecContext(ctx, `INSERT INTO provider_plugin_installations(installation_id,plugin_id,package_version,source_kind,source_reference,digest,manifest_json,verified_descriptor_json,status,installed_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		id, "dev.example.echo", "1.2.3", "local", "/source", digest, oldManifest, oldDescriptor, "disabled", now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	stableID, _ := installationdomain.ParseInstallationID(id)
	item, err := store.Get(ctx, stableID)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID() != stableID || item.PluginID().String() != "dev.example.echo" ||
		item.PackageVersion() != "1.2.3" || item.Digest() != digest ||
		item.ArtifactManifestName() != "mender-provider.json" ||
		item.Status() != installationdomain.StatusDisabled || !item.InstalledAt().Equal(now) {
		t.Fatalf("legacy installation identity mutated: %+v", item)
	}
	if item.Extension().Kind != platformplugin.ProviderTools {
		t.Fatalf("wrong kind: %s", item.Extension().Kind)
	}
	descriptor, ok := item.VerifiedDescriptor()
	if !ok || descriptor.ConfigVersion != 2 || descriptor.DisplayName != "Legacy Echo" || descriptor.Extension.ProtocolVersion != 1 {
		t.Fatalf("legacy descriptor not preserved: %+v, %v", descriptor, ok)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Restart cannot replay the migration or insert a second installation.
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	all, err := reopened.List(ctx)
	if err != nil || len(all) != 1 || all[0].ID() != stableID {
		t.Fatalf("idempotent reopen: %+v %v", all, err)
	}
	snapshots, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mender-phase27-backup-*.db"))
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("expected one original SQLite backup: %v %v", snapshots, err)
	}
	snapshotDB, err := sql.Open("sqlite", snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = snapshotDB.Close() }()
	var oldStoredManifest string
	if err := snapshotDB.QueryRowContext(ctx, "SELECT manifest_json FROM provider_plugin_installations WHERE installation_id=?", id).Scan(&oldStoredManifest); err != nil {
		t.Fatal(err)
	}
	if oldStoredManifest != oldManifest {
		t.Fatal("backup did not preserve original manifest")
	}
}

func TestLegacyInstallMigrationRejectsIncompatibleProtocolWithoutDroppingInventory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(ctx, `CREATE TABLE provider_plugin_installation_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL);
CREATE TABLE provider_plugin_installations(
installation_id TEXT PRIMARY KEY,plugin_id TEXT NOT NULL,package_version TEXT NOT NULL,source_kind TEXT NOT NULL,
source_reference TEXT NOT NULL,digest TEXT NOT NULL UNIQUE,manifest_json TEXT NOT NULL,
status TEXT NOT NULL,installed_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO provider_plugin_installation_schema_migrations(version,applied_at) VALUES(1,?)", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO provider_plugin_installations VALUES (?,?,?,?,?,?,?,?,?)`,
		"pinst_0123456789abcdef0123456789abcdef", "dev.example.echo", "1.2.3", "local", "/source", "sha256:"+strings.Repeat("a", 64),
		`{"manifestVersion":1,"pluginId":"dev.example.echo","version":"1.2.3","protocolVersions":[99],"displayName":"Broken","entrypoints":{"linux/amd64":"bin/echo"}}`,
		"enabled", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path)
	if err == nil {
		_ = s.Close()
		t.Fatal("accepted unconvertible legacy package")
	}
	verify, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = verify.Close() }()
	var oldCount int
	if err := verify.QueryRowContext(ctx, "SELECT COUNT(*) FROM provider_plugin_installations").Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 1 {
		t.Fatal("failed migration destroyed legacy record")
	}
}
