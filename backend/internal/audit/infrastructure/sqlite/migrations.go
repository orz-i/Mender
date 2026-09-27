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
	const latestSupportedVersion = 3
	if recordedVersion.Valid && recordedVersion.Int64 > latestSupportedVersion {
		return fmt.Errorf(
			"audit schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSupportedVersion,
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

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM audit_schema_migrations WHERE version = 2",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check audit migration 2: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(
			ctx,
			"ALTER TABLE invocation_audit ADD COLUMN error_summary TEXT NOT NULL DEFAULT ''",
		); err != nil {
			return fmt.Errorf("apply audit migration 2: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO audit_schema_migrations(version, applied_at) VALUES (2, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record audit migration 2: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM audit_schema_migrations WHERE version = 3",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check audit migration 3: %w", err)
	}
	if applied == 0 {
		for _, statement := range []string{
			"ALTER TABLE invocation_audit ADD COLUMN origin TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE invocation_audit ADD COLUMN scope_kind TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE invocation_audit ADD COLUMN scope_reference TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE invocation_audit ADD COLUMN outcome_kind TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE invocation_audit ADD COLUMN rejection_code TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE invocation_audit ADD COLUMN failure_source TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE invocation_audit ADD COLUMN failure_code TEXT NOT NULL DEFAULT ''",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply audit migration 3: %w", err)
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO audit_schema_migrations(version, applied_at) VALUES (3, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record audit migration 3: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit audit migrations: %w", err)
	}
	return nil
}
