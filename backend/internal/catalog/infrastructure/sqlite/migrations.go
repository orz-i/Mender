package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type migration struct {
	version    int
	statements []string
}

var catalogMigrations = []migration{
	{
		version: 1,
		statements: []string{
			`CREATE TABLE providers (
				id TEXT PRIMARY KEY,
				namespace TEXT NOT NULL UNIQUE,
				display_name TEXT NOT NULL,
				description TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE tools (
				id TEXT PRIMARY KEY,
				provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
				source_key TEXT NOT NULL,
				name TEXT NOT NULL,
				title TEXT NOT NULL,
				description TEXT NOT NULL,
				input_schema BLOB NOT NULL,
				output_schema BLOB,
				tags_json TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(provider_id, source_key)
			)`,
			`CREATE INDEX tools_provider_id_idx ON tools(provider_id)`,
			`CREATE VIRTUAL TABLE tools_fts USING fts5(
				tool_id UNINDEXED,
				provider_id UNINDEXED,
				name,
				title,
				description,
				tags
			)`,
		},
	},
}

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS catalog_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	for _, migration := range catalogMigrations {
		var count int
		if err := tx.QueryRowContext(
			ctx,
			"SELECT COUNT(*) FROM catalog_schema_migrations WHERE version = ?",
			migration.version,
		).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", migration.version, err)
		}
		if count != 0 {
			continue
		}

		for _, statement := range migration.statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply migration %d: %w", migration.version, err)
			}
		}

		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO catalog_schema_migrations(version, applied_at) VALUES (?, ?)",
			migration.version,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record migration %d: %w", migration.version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
