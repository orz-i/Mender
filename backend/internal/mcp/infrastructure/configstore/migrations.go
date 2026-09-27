package configstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin MCP config migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS mcp_config_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create MCP config migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM mcp_config_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check MCP config schema version: %w", err)
	}
	const latestSupportedVersion = 3
	if recordedVersion.Valid && recordedVersion.Int64 > latestSupportedVersion {
		return fmt.Errorf(
			"MCP config schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSupportedVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM mcp_config_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check MCP config migration 1: %w", err)
	}

	if applied == 0 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE mcp_provider_configs (
			provider_id TEXT PRIMARY KEY,
			transport TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			executable TEXT NOT NULL,
			args_json TEXT NOT NULL,
			directory TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("apply MCP config migration 1: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO mcp_config_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record MCP config migration 1: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM mcp_config_schema_migrations WHERE version = 2",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check MCP config migration 2: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(
			ctx,
			`ALTER TABLE mcp_provider_configs ADD COLUMN headers_json TEXT NOT NULL DEFAULT '{}'`,
		); err != nil {
			return fmt.Errorf("apply MCP config migration 2: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO mcp_config_schema_migrations(version, applied_at) VALUES (2, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record MCP config migration 2: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM mcp_config_schema_migrations WHERE version = 3",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check MCP config migration 3: %w", err)
	}
	if applied == 0 {
		statements := []string{
			`ALTER TABLE mcp_provider_configs ADD COLUMN credential_headers_json TEXT NOT NULL DEFAULT '[]'`,
			`ALTER TABLE mcp_provider_configs ADD COLUMN credential_environment_json TEXT NOT NULL DEFAULT '[]'`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply MCP config migration 3: %w", err)
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO mcp_config_schema_migrations(version, applied_at) VALUES (3, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record MCP config migration 3: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit MCP config migrations: %w", err)
	}
	return nil
}
