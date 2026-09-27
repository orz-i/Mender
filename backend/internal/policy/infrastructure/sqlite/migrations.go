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
		return fmt.Errorf("begin policy migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS policy_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create policy migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM policy_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check policy schema version: %w", err)
	}
	const latestSupportedVersion = 1
	if recordedVersion.Valid && recordedVersion.Int64 > latestSupportedVersion {
		return fmt.Errorf(
			"policy schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSupportedVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM policy_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check policy migration 1: %w", err)
	}
	if applied == 0 {
		statements := []string{
			`CREATE TABLE policy_settings (
				singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
				default_decision TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE policy_rules (
				rule_id TEXT PRIMARY KEY,
				display_name TEXT NOT NULL,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				decision TEXT NOT NULL,
				match_json TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply policy migration 1: %w", err)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO policy_settings(singleton_id, default_decision, updated_at) VALUES (1, 'allow', ?)",
			now,
		); err != nil {
			return fmt.Errorf("initialize policy settings: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO policy_schema_migrations(version, applied_at) VALUES (1, ?)",
			now,
		); err != nil {
			return fmt.Errorf("record policy migration 1: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit policy migrations: %w", err)
	}
	return nil
}
