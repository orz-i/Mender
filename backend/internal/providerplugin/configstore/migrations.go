package configstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 2

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin provider config migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS provider_config_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create provider config migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM provider_config_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check provider config schema version: %w", err)
	}
	if recordedVersion.Valid && recordedVersion.Int64 > latestSchemaVersion {
		return fmt.Errorf(
			"provider config schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSchemaVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM provider_config_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check provider config migration 1: %w", err)
	}
	if applied == 0 {
		statements := []string{
			`CREATE TABLE provider_configs (
				provider_id TEXT PRIMARY KEY,
				plugin_id TEXT NOT NULL,
				config_version INTEGER NOT NULL,
				revision TEXT NOT NULL,
				config_json TEXT NOT NULL
			)`,
			`CREATE TABLE provider_config_imports (
				name TEXT PRIMARY KEY,
				applied_at TEXT NOT NULL
			)`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply provider config migration 1: %w", err)
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO provider_config_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record provider config migration 1: %w", err)
		}
	}

	var appliedV2 int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM provider_config_schema_migrations WHERE version = 2",
	).Scan(&appliedV2); err != nil {
		return fmt.Errorf("check provider config migration 2: %w", err)
	}
	if appliedV2 == 0 {
		if _, err := tx.ExecContext(
			ctx,
			"ALTER TABLE provider_configs ADD COLUMN plugin_installation_id TEXT",
		); err != nil {
			return fmt.Errorf("apply provider config migration 2: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO provider_config_schema_migrations(version, applied_at) VALUES (2, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record provider config migration 2: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider config migrations: %w", err)
	}
	return nil
}
