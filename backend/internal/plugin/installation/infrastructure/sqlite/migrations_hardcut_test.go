package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const retiredInventorySchema = `
CREATE TABLE provider_plugin_installations(
	installation_id TEXT PRIMARY KEY,
	plugin_id TEXT NOT NULL,
	package_version TEXT NOT NULL,
	source_kind TEXT NOT NULL,
	source_reference TEXT NOT NULL,
	digest TEXT NOT NULL UNIQUE,
	manifest_json TEXT NOT NULL,
	status TEXT NOT NULL,
	installed_at TEXT NOT NULL
);`

const retiredInventoryRow = `{"manifestVersion":1,"pluginId":"dev.example.echo","version":"1.2.3","protocolVersions":[1],"displayName":"Retired Echo","entrypoints":{"darwin/arm64":"bin/echo"}}`

func TestRetiredProviderInventoryFailsClosedAndPreservesOriginalRows(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"data-and-marker", "old-data-only", "old-marker-only", "mixed-old-and-modern"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "mender.db")
			if mode == "mixed-old-and-modern" {
				modern, err := Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				kept := testInstallation(t, "pinst_000000000000000000000000000000ee", "dev.example.keep", "2.0.0", "b")
				if err := modern.Create(ctx, kept); err != nil {
					t.Fatal(err)
				}
				if err := modern.Close(); err != nil {
					t.Fatal(err)
				}
			}
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			createData := mode != "old-marker-only"
			createMarker := mode != "old-data-only"
			if createMarker {
				if _, err := raw.ExecContext(ctx, `CREATE TABLE provider_plugin_installation_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
					t.Fatal(err)
				}
				if _, err := raw.ExecContext(ctx, `INSERT INTO provider_plugin_installation_schema_migrations VALUES(2,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
					t.Fatal(err)
				}
			}
			if createData {
				if _, err := raw.ExecContext(ctx, retiredInventorySchema); err != nil {
					t.Fatal(err)
				}
				if _, err := raw.ExecContext(ctx, `INSERT INTO provider_plugin_installations VALUES(?,?,?,?,?,?,?,?,?)`,
					"pinst_0123456789abcdef0123456789abcdef", "dev.example.echo", "1.2.3",
					"local", "/original/source", "sha256:"+strings.Repeat("a", 64),
					retiredInventoryRow, "disabled", "2026-09-30T11:00:00Z"); err != nil {
					t.Fatal(err)
				}
			}
			if err := raw.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := Open(ctx, path)
			if err == nil {
				_ = store.Close()
				t.Fatal("silently imported unsupported Provider-v1 installation inventory")
			}
			if !strings.Contains(err.Error(), "retired Provider v1 inventory/schema is unsupported") {
				t.Fatalf("retired inventory rejected without actionable diagnostic: %v", err)
			}
			verify, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = verify.Close() }()
			if createData {
				var source, manifest, digest string
				if err := verify.QueryRowContext(ctx, `SELECT source_reference,manifest_json,digest FROM provider_plugin_installations WHERE installation_id='pinst_0123456789abcdef0123456789abcdef'`).Scan(&source, &manifest, &digest); err != nil {
					t.Fatal(err)
				}
				if source != "/original/source" || manifest != retiredInventoryRow || digest != "sha256:"+strings.Repeat("a", 64) {
					t.Fatal("retired installation data was altered on refused startup")
				}
			}
			if createMarker {
				var version int
				if err := verify.QueryRowContext(ctx, `SELECT MAX(version) FROM provider_plugin_installation_schema_migrations`).Scan(&version); err != nil || version != 2 {
					t.Fatal("retired schema marker was rewritten")
				}
			}
			var modernTable, versionTable int
			if err := verify.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='plugin_installations'`).Scan(&modernTable); err != nil {
				t.Fatal(err)
			}
			if err := verify.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='plugin_installation_schema_migrations'`).Scan(&versionTable); err != nil {
				t.Fatal(err)
			}
			if mode == "mixed-old-and-modern" {
				if modernTable != 1 || versionTable != 1 {
					t.Fatal("mixed-schema rejection dropped modern inventory")
				}
				var pluginID, packageVersion, status string
				if err := verify.QueryRowContext(ctx, `SELECT plugin_id,package_version,status FROM plugin_installations WHERE installation_id='pinst_000000000000000000000000000000ee'`).Scan(&pluginID, &packageVersion, &status); err != nil || pluginID != "dev.example.keep" || packageVersion != "2.0.0" || status != "enabled" {
					t.Fatalf("rejected legacy schema altered valid modern installation identity: %q %q %q %v", pluginID, packageVersion, status, err)
				}
			} else if modernTable != 0 || versionTable != 0 {
				t.Fatal("retired schema rejection created modern inventory or migration markers")
			}
			backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mender-phase27-backup-*.db"))
			if err != nil || len(backups) != 0 {
				t.Fatalf("obsolete converter created a backup: %v", backups)
			}
			var integrity string
			if err := verify.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil || integrity != "ok" {
				t.Fatal("retired schema rejection damaged SQLite database")
			}
		})
	}
}
