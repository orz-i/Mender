package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSupportedVersion = 2

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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tunnel migrations: %w", err)
	}
	return nil
}
