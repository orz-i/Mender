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
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS audit_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create audit migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM audit_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check audit schema version: %w", err)
	}
	if recordedVersion.Valid && recordedVersion.Int64 > 1 {
		return fmt.Errorf(
			"audit schema version %d is newer than supported version 1",
			recordedVersion.Int64,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM audit_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check audit migration 1: %w", err)
	}

	if applied == 0 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE invocation_audit (
			invocation_id TEXT PRIMARY KEY,
			tool_id TEXT NOT NULL,
			provider_id TEXT,
			caller TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			policy_kind TEXT NOT NULL,
			policy_reason TEXT NOT NULL,
			approval TEXT NOT NULL,
			status TEXT NOT NULL,
			result_kind TEXT NOT NULL,
			error_class TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("apply audit migration 1: %w", err)
		}

		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO audit_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record audit migration 1: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit audit migrations: %w", err)
	}
	return nil
}
