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
	{
		version: 2,
		statements: []string{
			`CREATE TABLE catalog_tool_revision (
				id INTEGER PRIMARY KEY CHECK(id=1),
				revision INTEGER NOT NULL CHECK(revision>=0)
			)`,
			`INSERT INTO catalog_tool_revision(id,revision) VALUES(1,0)`,
			`CREATE TRIGGER catalog_tool_insert_revision AFTER INSERT ON tools
			 BEGIN UPDATE catalog_tool_revision SET revision=revision+1 WHERE id=1; END`,
			`CREATE TRIGGER catalog_tool_update_revision AFTER UPDATE ON tools
			 BEGIN UPDATE catalog_tool_revision SET revision=revision+1 WHERE id=1; END`,
			`CREATE TRIGGER catalog_tool_delete_revision AFTER DELETE ON tools
			 BEGIN UPDATE catalog_tool_revision SET revision=revision+1 WHERE id=1; END`,
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

	latestVersion := 0
	if len(catalogMigrations) != 0 {
		latestVersion = catalogMigrations[len(catalogMigrations)-1].version
	}
	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM catalog_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check catalog schema version: %w", err)
	}
	if recordedVersion.Valid && recordedVersion.Int64 > int64(latestVersion) {
		return fmt.Errorf(
			"catalog schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestVersion,
		)
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
