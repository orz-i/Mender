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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit profile migrations: %w", err)
	}
	return nil
}
