package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const currentSchemaVersion = 2

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin skill installation migrations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(
		ctx,
		"CREATE TABLE IF NOT EXISTS skill_installation_schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)",
	); err != nil {
		return fmt.Errorf("create skill installation migration table: %w", err)
	}

	var version int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COALESCE(MAX(version), 0) FROM skill_installation_schema_migrations",
	).Scan(&version); err != nil {
		return fmt.Errorf("read skill installation schema version: %w", err)
	}
	if version > currentSchemaVersion {
		return fmt.Errorf(
			"skill installation schema version %d is newer than supported version %d",
			version,
			currentSchemaVersion,
		)
	}
	if version < 1 {
		createTable := "CREATE TABLE skill_installations (" +
			"installation_id TEXT PRIMARY KEY, " +
			"skill_id TEXT NOT NULL, " +
			"name TEXT NOT NULL, " +
			"description TEXT NOT NULL, " +
			"source_kind TEXT NOT NULL, " +
			"source_reference TEXT NOT NULL, " +
			"digest TEXT NOT NULL, " +
			"version_hint TEXT NOT NULL, " +
			"trusted INTEGER NOT NULL CHECK (trusted IN (0, 1)), " +
			"enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)), " +
			"installed_at TEXT NOT NULL, " +
			"UNIQUE(skill_id, digest), " +
			"CHECK (enabled = 0 OR trusted = 1))"
		if _, err := tx.ExecContext(ctx, createTable); err != nil {
			return fmt.Errorf("apply skill installation migration 1: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO skill_installation_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record skill installation migration 1: %w", err)
		}
		version = 1
	}
	if version < 2 {
		createDeployments := "CREATE TABLE skill_deployments (" +
			"deployment_id TEXT PRIMARY KEY, " +
			"installation_id TEXT NOT NULL, " +
			"target TEXT NOT NULL, " +
			"root TEXT NOT NULL, " +
			"destination TEXT NOT NULL UNIQUE, " +
			"digest TEXT NOT NULL, " +
			"deployed_at TEXT NOT NULL)"
		if _, err := tx.ExecContext(ctx, createDeployments); err != nil {
			return fmt.Errorf("apply skill installation migration 2: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO skill_installation_schema_migrations(version, applied_at) VALUES (2, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record skill installation migration 2: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit skill installation migrations: %w", err)
	}
	return nil
}
