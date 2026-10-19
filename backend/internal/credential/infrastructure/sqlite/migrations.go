package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin credential migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS credential_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create credential migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM credential_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check credential schema version: %w", err)
	}
	const latestSupportedVersion = 2
	if recordedVersion.Valid && recordedVersion.Int64 > latestSupportedVersion {
		return fmt.Errorf(
			"credential schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSupportedVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM credential_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check credential migration 1: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE credentials (
			reference TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			source TEXT NOT NULL,
			environment_variable TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("apply credential migration 1: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO credential_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record credential migration 1: %w", err)
		}
	}

	var appliedV2 int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM credential_schema_migrations WHERE version = 2").Scan(&appliedV2); err != nil {
		return fmt.Errorf("check credential migration 2: %w", err)
	}
	if appliedV2 == 0 {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE credentials ADD COLUMN oauth_metadata TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("apply credential migration 2: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO credential_schema_migrations(version, applied_at) VALUES (2, ?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record credential migration 2: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit credential migrations: %w", err)
	}
	return nil
}
