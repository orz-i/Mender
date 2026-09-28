package sqlite

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
		return fmt.Errorf("begin plugin installation migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS provider_plugin_installation_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create plugin installation migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM provider_plugin_installation_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check plugin installation schema version: %w", err)
	}
	if recordedVersion.Valid && recordedVersion.Int64 > latestSchemaVersion {
		return fmt.Errorf(
			"plugin installation schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSchemaVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM provider_plugin_installation_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check plugin installation migration 1: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE provider_plugin_installations (
			installation_id TEXT PRIMARY KEY,
			plugin_id TEXT NOT NULL,
			package_version TEXT NOT NULL,
			source_kind TEXT NOT NULL,
			source_reference TEXT NOT NULL,
			digest TEXT NOT NULL UNIQUE,
			manifest_json TEXT NOT NULL,
			status TEXT NOT NULL,
			installed_at TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("apply plugin installation migration 1: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO provider_plugin_installation_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record plugin installation migration 1: %w", err)
		}
	}

	var appliedV2 int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM provider_plugin_installation_schema_migrations WHERE version = 2",
	).Scan(&appliedV2); err != nil {
		return fmt.Errorf("check plugin installation migration 2: %w", err)
	}
	if appliedV2 == 0 {
		if _, err := tx.ExecContext(
			ctx,
			"ALTER TABLE provider_plugin_installations ADD COLUMN verified_descriptor_json TEXT",
		); err != nil {
			return fmt.Errorf("apply plugin installation migration 2: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO provider_plugin_installation_schema_migrations(version, applied_at) VALUES (2, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record plugin installation migration 2: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit plugin installation migrations: %w", err)
	}
	return nil
}
