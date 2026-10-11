package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSupportedVersion = 3

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tunnel migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tunnel_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create tunnel migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM tunnel_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check tunnel schema version: %w", err)
	}
	if recordedVersion.Valid && recordedVersion.Int64 > latestSupportedVersion {
		return fmt.Errorf(
			"tunnel schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSupportedVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM tunnel_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check tunnel migration 1: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE tunnels (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			provider TEXT NOT NULL,
			external_tunnel_id TEXT NOT NULL,
			runtime_credential_ref TEXT NOT NULL,
			executable TEXT NOT NULL,
			UNIQUE(provider, external_tunnel_id)
		)`); err != nil {
			return fmt.Errorf("apply tunnel migration 1: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO tunnel_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record tunnel migration 1: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM tunnel_schema_migrations WHERE version = 2",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check tunnel migration 2: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE tunnels DROP COLUMN executable"); err != nil {
			return fmt.Errorf("apply tunnel migration 2: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO tunnel_schema_migrations(version, applied_at) VALUES (2, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record tunnel migration 2: %w", err)
		}
	}

	var pinned int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tunnel_schema_migrations WHERE version=3").Scan(&pinned); err != nil {
		return err
	}
	if pinned == 0 {
		for _, statement := range []string{
			"ALTER TABLE tunnels ADD COLUMN plugin_id TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE tunnels ADD COLUMN extension_id TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE tunnels ADD COLUMN installation_id TEXT NOT NULL DEFAULT ''",
			`UPDATE tunnels SET plugin_id='io.mender.tunnel.openai',extension_id='main'
                WHERE provider='openai'`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("tunnel migration 3 extension binding: %w", err)
			}
		}
		var invalid int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tunnels WHERE plugin_id='' OR extension_id!='main' OR installation_id!=''`).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("tunnel migration 3: %d unsupported legacy tunnel providers", invalid)
		}
		for _, statement := range []string{
			`CREATE TRIGGER tunnel_extension_binding_insert BEFORE INSERT ON tunnels
            WHEN NEW.plugin_id='' OR NEW.extension_id!='main' OR
                (NEW.provider='openai' AND (NEW.plugin_id!='io.mender.tunnel.openai' OR NEW.installation_id!='')) OR
                (NEW.provider='external' AND (NEW.installation_id='' OR NEW.plugin_id='io.mender.tunnel.openai')) OR
                NEW.provider NOT IN ('openai','external')
            BEGIN SELECT RAISE(ABORT,'Tunnel extension binding is invalid'); END`,
			`CREATE TRIGGER tunnel_extension_binding_update BEFORE UPDATE ON tunnels
            WHEN NEW.provider!=OLD.provider OR NEW.plugin_id!=OLD.plugin_id OR
                NEW.extension_id!=OLD.extension_id OR NEW.installation_id!=OLD.installation_id OR
                NEW.runtime_credential_ref!=OLD.runtime_credential_ref OR NEW.external_tunnel_id!=OLD.external_tunnel_id
            BEGIN SELECT RAISE(ABORT,'Tunnel extension and Credential binding are immutable'); END`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("tunnel migration 3 guard: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO tunnel_schema_migrations(version,applied_at) VALUES(3,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tunnel migrations: %w", err)
	}
	return nil
}
