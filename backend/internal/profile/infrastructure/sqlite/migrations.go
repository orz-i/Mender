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
		return fmt.Errorf("begin profile migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS profile_schema_migrations (
        version INTEGER PRIMARY KEY,
        applied_at TEXT NOT NULL
    )`); err != nil {
		return fmt.Errorf("create profile migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM profile_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check profile schema version: %w", err)
	}
	const latestSupportedVersion = 5
	if recordedVersion.Valid && recordedVersion.Int64 > latestSupportedVersion {
		return fmt.Errorf(
			"profile schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSupportedVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM profile_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check profile migration 1: %w", err)
	}

	if applied == 0 {
		statements := []string{
			`CREATE TABLE profiles (
                id TEXT PRIMARY KEY,
                slug TEXT NOT NULL UNIQUE,
                display_name TEXT NOT NULL,
                exposure_mode TEXT NOT NULL
            )`,
			`CREATE TABLE profile_tools (
                profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
                tool_id TEXT NOT NULL,
                direct INTEGER NOT NULL CHECK (direct IN (0, 1)),
                PRIMARY KEY(profile_id, tool_id)
            )`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply profile migration 1: %w", err)
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO profile_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record profile migration 1: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM profile_schema_migrations WHERE version = 2",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check profile migration 2: %w", err)
	}
	if applied == 0 {
		statements := []string{
			"ALTER TABLE profile_tools ADD COLUMN alias TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE profile_tools ADD COLUMN title_override TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE profile_tools ADD COLUMN description_override TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE profile_tools ADD COLUMN policy_constraint TEXT NOT NULL DEFAULT 'inherit'",
			"ALTER TABLE profile_tools ADD COLUMN concurrency_constraint TEXT NOT NULL DEFAULT 'inherit'",
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply profile migration 2: %w", err)
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO profile_schema_migrations(version, applied_at) VALUES (2, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record profile migration 2: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM profile_schema_migrations WHERE version = 3",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check profile migration 3: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(
			ctx,
			"ALTER TABLE profile_tools ADD COLUMN credential_refs_json TEXT NOT NULL DEFAULT '[]'",
		); err != nil {
			return fmt.Errorf("apply profile migration 3: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO profile_schema_migrations(version, applied_at) VALUES (3, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record profile migration 3: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM profile_schema_migrations WHERE version = 4",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check profile migration 4: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(
			ctx,
			"ALTER TABLE profiles ADD COLUMN tunnel_id TEXT NOT NULL DEFAULT ''",
		); err != nil {
			return fmt.Errorf("apply profile migration 4: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO profile_schema_migrations(version, applied_at) VALUES (4, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record profile migration 4: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM profile_schema_migrations WHERE version = 5").Scan(&applied); err != nil {
		return fmt.Errorf("check profile migration 5: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE profiles ADD COLUMN revision INTEGER NOT NULL DEFAULT 0 CHECK(revision >= 0)"); err != nil {
			return fmt.Errorf("apply profile migration 5: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO profile_schema_migrations(version, applied_at) VALUES (5, ?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record profile migration 5: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit profile migrations: %w", err)
	}
	return nil
}
