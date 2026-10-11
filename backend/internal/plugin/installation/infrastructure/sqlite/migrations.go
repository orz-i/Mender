package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	legacymanifest "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/sqlite/legacy"
	genericmanifest "github.com/orz-i/Mender/backend/internal/plugin/manifest"
)

const latestSchemaVersion = 1

// backupLegacy takes a SQLite-consistent snapshot including WAL contents before
// any forward-only installation schema hard cut.
func backupLegacy(ctx context.Context, db *sql.DB, databasePath string) error {
	var found int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='provider_plugin_installations'").Scan(&found); err != nil {
		return err
	}
	if found == 0 {
		return nil
	}
	directory := filepath.Dir(databasePath)
	file, err := os.CreateTemp(directory, ".mender-phase27-backup-*.db")
	if err != nil {
		return fmt.Errorf("create phase27 backup destination: %w", err)
	}
	backupPath := file.Name()
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", backupPath); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("create phase27 pre-upgrade SQLite snapshot: %w", err)
	}
	return nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin plugin installation migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS plugin_installation_schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	var recorded sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM plugin_installation_schema_migrations").Scan(&recorded); err != nil {
		return err
	}
	if recorded.Valid && recorded.Int64 > latestSchemaVersion {
		return fmt.Errorf("plugin installation schema version %d is newer than supported version %d", recorded.Int64, latestSchemaVersion)
	}
	var oldCount, newCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='provider_plugin_installations'").Scan(&oldCount); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='plugin_installations'").Scan(&newCount); err != nil {
		return err
	}
	if oldCount != 0 && newCount != 0 {
		return fmt.Errorf("plugin installation migration: both legacy and new inventories exist")
	}
	if oldCount != 0 {
		var oldVersion sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM provider_plugin_installation_schema_migrations").Scan(&oldVersion); err != nil {
			return fmt.Errorf("plugin installation legacy migration state: %w", err)
		}
		if oldVersion.Valid && oldVersion.Int64 > 2 {
			return fmt.Errorf("legacy plugin installation schema version %d is newer than supported", oldVersion.Int64)
		}
	}
	if newCount == 0 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE plugin_installations (
			installation_id TEXT PRIMARY KEY,
			plugin_id TEXT NOT NULL,
			package_version TEXT NOT NULL,
			source_kind TEXT NOT NULL,
			source_reference TEXT NOT NULL,
			digest TEXT NOT NULL UNIQUE,
			manifest_json TEXT NOT NULL,
			artifact_manifest_file TEXT NOT NULL,
			verified_descriptor_json TEXT,
			status TEXT NOT NULL,
			installed_at TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("create generic plugin inventory: %w", err)
		}
	}
	if oldCount != 0 {
		if err := transferLegacyProviderInstallations(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DROP TABLE provider_plugin_installations"); err != nil {
			return fmt.Errorf("remove migrated legacy plugin inventory: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DROP TABLE provider_plugin_installation_schema_migrations"); err != nil {
			return fmt.Errorf("remove legacy plugin schema markers: %w", err)
		}
	}
	if !recorded.Valid {
		if _, err := tx.ExecContext(ctx, "INSERT INTO plugin_installation_schema_migrations(version,applied_at) VALUES (?,?)", latestSchemaVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit generic plugin installation migrations: %w", err)
	}
	return nil
}

// transferLegacyProviderInstallations is the ONLY v1 Provider manifest decoder.
// Normal catalogue reads and package launch paths consume the migrated generic
// descriptor and verify original package bytes, not legacy configuration.
func transferLegacyProviderInstallations(ctx context.Context, tx *sql.Tx) error {
	columns, err := tx.QueryContext(ctx, "PRAGMA table_info(provider_plugin_installations)")
	if err != nil {
		return err
	}
	hasDescriptor := false
	for columns.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def sql.NullString
		if err := columns.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			_ = columns.Close()
			return err
		}
		if name == "verified_descriptor_json" {
			hasDescriptor = true
		}
	}
	if err := columns.Close(); err != nil {
		return err
	}
	verified := "NULL"
	if hasDescriptor {
		verified = "verified_descriptor_json"
	}
	rows, err := tx.QueryContext(ctx, `SELECT installation_id,plugin_id,package_version,source_kind,source_reference,digest,manifest_json,`+verified+`,status,installed_at FROM provider_plugin_installations ORDER BY installation_id`)
	if err != nil {
		return err
	}
	type legacyRow struct {
		id, pluginID, version, sourceKind, source, digest, manifestJSON, status, installedAt string
		verified                                                                             sql.NullString
	}
	var entries []legacyRow
	for rows.Next() {
		var x legacyRow
		if err := rows.Scan(&x.id, &x.pluginID, &x.version, &x.sourceKind, &x.source, &x.digest, &x.manifestJSON, &x.verified, &x.status, &x.installedAt); err != nil {
			_ = rows.Close()
			return err
		}
		entries = append(entries, x)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, old := range entries {
		legacy, err := legacymanifest.Parse([]byte(old.manifestJSON))
		if err != nil {
			return fmt.Errorf("migrate plugin %s legacy manifest: %w", old.id, err)
		}
		if legacy.PluginID != old.pluginID || legacy.Version != old.version {
			return fmt.Errorf("migrate plugin %s: legacy identity mismatch", old.id)
		}
		protocol := uint32(0)
		for _, v := range legacy.ProtocolVersions {
			if v == 1 {
				protocol = v
				break
			}
		}
		if protocol == 0 {
			return fmt.Errorf("migrate plugin %s: no supported Provider protocol v1", old.id)
		}
		generic := genericmanifest.Manifest{
			ManifestVersion: genericmanifest.CurrentVersion,
			PluginID:        legacy.PluginID, Version: legacy.Version,
			DisplayName: legacy.DisplayName, Description: legacy.Description,
			Entrypoints: legacy.Entrypoints,
			Extensions:  []platformplugin.Extension{{ID: "main", Kind: platformplugin.ProviderTools, ProtocolVersion: protocol}},
		}
		raw, err := json.Marshal(generic)
		if err != nil {
			return err
		}
		id, err := installationdomain.ParseInstallationID(old.id)
		if err != nil {
			return err
		}
		pid, err := platformplugin.ParseID(old.pluginID)
		if err != nil {
			return err
		}
		installedAt, err := time.Parse(time.RFC3339Nano, old.installedAt)
		if err != nil {
			return err
		}
		item, err := installationdomain.NewInstallation(id, pid, old.version, installationdomain.SourceKind(old.sourceKind), old.source, old.digest, raw, installationdomain.Status(old.status), installedAt)
		if err != nil {
			return fmt.Errorf("migrate plugin %s: %w", old.id, err)
		}
		item, err = item.WithArtifactManifestName("mender-provider.json")
		if err != nil {
			return err
		}
		var descriptor any
		if old.verified.Valid {
			d, err := decodeLegacyProviderDescriptor(old.verified.String, generic.Extensions[0])
			if err != nil {
				return fmt.Errorf("migrate plugin %s verified descriptor: %w", old.id, err)
			}
			item, err = item.WithVerifiedDescriptor(d)
			if err != nil {
				return fmt.Errorf("migrate plugin %s: %w", old.id, err)
			}
			descriptor, err = marshalVerifiedDescriptor(item)
			if err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO plugin_installations (
			installation_id,plugin_id,package_version,source_kind,source_reference,digest,
			manifest_json,artifact_manifest_file,verified_descriptor_json,status,installed_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			item.ID().String(), item.PluginID().String(), item.PackageVersion(), string(item.SourceKind()),
			item.SourceReference(), item.Digest(), string(item.ManifestJSON()), item.ArtifactManifestName(),
			descriptor, string(item.Status()), item.InstalledAt().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("migrate plugin %s: %w", old.id, err)
		}
	}
	return nil
}
