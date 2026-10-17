package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 1

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
	// Hard cut: historical Provider-v1 installation tables cannot be
	// automatically imported into the generic installation inventory.
	// Detect BOTH the old data table and its migration marker, including
	// empty tables or mixed old/new schemas. This transaction rolls back
	// without altering existing user rows, installation IDs, or schema.
	var retiredCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name IN
		('provider_plugin_installations','provider_plugin_installation_schema_migrations')`).Scan(&retiredCount); err != nil {
		return fmt.Errorf("inspect retired Provider installation schema: %w", err)
	}
	if retiredCount != 0 {
		return fmt.Errorf("plugin installation: retired Provider v1 inventory/schema is unsupported; migrate offline with a prior release")
	}
	var modernCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='plugin_installations'").Scan(&modernCount); err != nil {
		return err
	}
	if modernCount == 0 {
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
	if !recorded.Valid {
		if _, err := tx.ExecContext(ctx, "INSERT INTO plugin_installation_schema_migrations(version,applied_at) VALUES (?,?)",
			latestSchemaVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	// The runtime owns one SQLite file for both bounded contexts. Protect
	// the immutable Sandbox -> Plugin Installation pin INSIDE SQLite, not only
	// through a racy check-then-disable sequence in the management layer.
	// Sandbox migrations run first in normal Runtime composition; standalone
	// plugin-store tests have no Sandbox inventory and skip these guards.
	var sandboxTable int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sandboxes'").Scan(&sandboxTable); err != nil {
		return err
	}
	if sandboxTable != 0 {
		for _, statement := range []string{
			`CREATE TRIGGER IF NOT EXISTS sandbox_external_installation_pin_insert
            BEFORE INSERT ON sandboxes WHEN NEW.backend='external'
            AND NOT EXISTS(
              SELECT 1 FROM plugin_installations pi
              WHERE pi.installation_id=NEW.backend_installation_id
                AND pi.plugin_id=NEW.backend_plugin_id
                AND pi.status='enabled'
                AND json_extract(pi.manifest_json,'$.extensions[0].kind')='sandbox.backend'
                AND pi.verified_descriptor_json IS NOT NULL
            )
            BEGIN SELECT RAISE(ABORT,'external Sandbox installation must be enabled and verified'); END`,
			`CREATE TRIGGER IF NOT EXISTS sandbox_driver_disable_guard
            BEFORE UPDATE OF status ON plugin_installations
            WHEN NEW.status!='enabled' AND EXISTS(
              SELECT 1 FROM sandboxes s WHERE s.backend='external'
                AND s.backend_installation_id=OLD.installation_id
            )
            BEGIN SELECT RAISE(ABORT,'Sandbox driver installation is in use'); END`,
			`CREATE TRIGGER IF NOT EXISTS sandbox_driver_remove_guard
            BEFORE DELETE ON plugin_installations
            WHEN EXISTS(
              SELECT 1 FROM sandboxes s WHERE s.backend='external'
                AND s.backend_installation_id=OLD.installation_id
            )
            BEGIN SELECT RAISE(ABORT,'Sandbox driver installation is in use'); END`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("install cross-domain Sandbox driver safety guards: %w", err)
			}
		}
	}
	// A Tunnel transport may not be disabled or removed while a configured
	// Tunnel still holds its exact InstallationID. SQLite closes concurrent
	// resource-create vs management-status TOCTOU races.
	var tunnelsPresent int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tunnels'").Scan(&tunnelsPresent); err != nil {
		return err
	}
	if tunnelsPresent != 0 {
		for _, statement := range []string{
			`CREATE TRIGGER IF NOT EXISTS tunnel_transport_install_pin_insert
            BEFORE INSERT ON tunnels WHEN NEW.provider='external'
            AND NOT EXISTS(
                SELECT 1 FROM plugin_installations pi
                WHERE pi.installation_id=NEW.installation_id AND pi.plugin_id=NEW.plugin_id
                    AND pi.status='enabled' AND pi.verified_descriptor_json IS NOT NULL
                    AND json_extract(pi.manifest_json,'$.extensions[0].kind')='tunnel.transport'
            ) BEGIN SELECT RAISE(ABORT,'Tunnel plugin installation unavailable'); END`,
			`CREATE TRIGGER IF NOT EXISTS tunnel_transport_disable_guard
            BEFORE UPDATE OF status ON plugin_installations
            WHEN NEW.status!='enabled' AND EXISTS(
                SELECT 1 FROM tunnels t WHERE t.provider='external'
                  AND t.installation_id=OLD.installation_id
            ) BEGIN SELECT RAISE(ABORT,'Tunnel transport installation is in use'); END`,
			`CREATE TRIGGER IF NOT EXISTS tunnel_transport_remove_guard
            BEFORE DELETE ON plugin_installations
            WHEN EXISTS(
                SELECT 1 FROM tunnels t WHERE t.provider='external'
                  AND t.installation_id=OLD.installation_id
            ) BEGIN SELECT RAISE(ABORT,'Tunnel transport installation is in use'); END`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("tunnel plugin installation reference guard: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit generic plugin installation migrations: %w", err)
	}
	return nil
}
