package configstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 4

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin provider config migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS provider_config_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create provider config migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM provider_config_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check provider config schema version: %w", err)
	}
	if recordedVersion.Valid && recordedVersion.Int64 > latestSchemaVersion {
		return fmt.Errorf(
			"provider config schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSchemaVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM provider_config_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check provider config migration 1: %w", err)
	}
	if applied == 0 {
		statements := []string{
			`CREATE TABLE provider_configs (
				provider_id TEXT PRIMARY KEY,
				plugin_id TEXT NOT NULL,
				config_version INTEGER NOT NULL,
				revision TEXT NOT NULL,
				config_json TEXT NOT NULL
			)`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply provider config migration 1: %w", err)
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO provider_config_schema_migrations(version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record provider config migration 1: %w", err)
		}
	}

	var appliedV2 int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM provider_config_schema_migrations WHERE version = 2",
	).Scan(&appliedV2); err != nil {
		return fmt.Errorf("check provider config migration 2: %w", err)
	}
	if appliedV2 == 0 {
		if _, err := tx.ExecContext(
			ctx,
			"ALTER TABLE provider_configs ADD COLUMN plugin_installation_id TEXT",
		); err != nil {
			return fmt.Errorf("apply provider config migration 2: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO provider_config_schema_migrations(version, applied_at) VALUES (2, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record provider config migration 2: %w", err)
		}
	}

	var appliedV3 int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM provider_config_schema_migrations WHERE version = 3",
	).Scan(&appliedV3); err != nil {
		return fmt.Errorf("check provider config migration 3: %w", err)
	}
	if appliedV3 == 0 {
		statements := []string{
			"ALTER TABLE provider_configs ADD COLUMN credential_refs_json TEXT",
			"ALTER TABLE provider_configs ADD COLUMN connection_summary TEXT",
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply provider config migration 3: %w", err)
			}
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO provider_config_schema_migrations(version, applied_at) VALUES (3, ?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record provider config migration 3: %w", err)
		}
	}

	// V4 is an intentional hard cut: only canonical Provider Configs survive.
	// Do not import missing MCP adapter rows or translate Workspace v1 configs.
	// If a source row has no authoritative canonical counterpart, roll back
	// everything so an operator can inspect the pre-upgrade backup explicitly.
	var appliedV4 int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM provider_config_schema_migrations WHERE version = 4",
	).Scan(&appliedV4); err != nil {
		return fmt.Errorf("check provider config migration 4: %w", err)
	}
	if appliedV4 == 0 {
		var obsoleteMCPTable int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='mcp_provider_configs'",
		).Scan(&obsoleteMCPTable); err != nil {
			return fmt.Errorf("check retired MCP table: %w", err)
		}
		if obsoleteMCPTable != 0 {
			var unmapped int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)
				FROM mcp_provider_configs legacy
				LEFT JOIN provider_configs canonical ON canonical.provider_id=legacy.provider_id
				WHERE canonical.provider_id IS NULL
				OR canonical.plugin_id!='io.mender.provider.mcp'
				OR canonical.plugin_installation_id IS NOT NULL
				OR canonical.config_version!=1
				OR NOT json_valid(canonical.config_json)
				OR COALESCE(json_extract(CASE WHEN json_valid(canonical.config_json) THEN canonical.config_json ELSE '{}' END, '$.transport.kind'),'') NOT IN ('streamable-http','stdio')`,
			).Scan(&unmapped); err != nil {
				return fmt.Errorf("validate retired MCP configurations: %w", err)
			}
			if unmapped != 0 {
				return fmt.Errorf("retire MCP compatibility: %d rows have no valid canonical MCP Provider configuration", unmapped)
			}
		}
		var oldWorkspace int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM provider_configs
			WHERE plugin_id='io.mender.provider.workspace' AND plugin_installation_id IS NULL AND config_version!=2`,
		).Scan(&oldWorkspace); err != nil {
			return fmt.Errorf("validate Workspace v2-only Provider configs: %w", err)
		}
		if oldWorkspace != 0 {
			return fmt.Errorf("retire Workspace v1 compatibility: %d Providers are not v2-only", oldWorkspace)
		}
		for _, statement := range []string{
			"DROP TABLE IF EXISTS mcp_provider_configs",
			"DROP TABLE IF EXISTS mcp_config_schema_migrations",
			"DROP TABLE IF EXISTS provider_config_imports",
			`CREATE TRIGGER IF NOT EXISTS workspace_v2_only_provider_config_insert
				BEFORE INSERT ON provider_configs
				WHEN NEW.plugin_id='io.mender.provider.workspace' AND NEW.plugin_installation_id IS NULL AND NEW.config_version!=2
				BEGIN SELECT RAISE(ABORT,'Workspace Provider v1 retired'); END`,
			`CREATE TRIGGER IF NOT EXISTS workspace_v2_only_provider_config_update
				BEFORE UPDATE OF plugin_id,plugin_installation_id,config_version ON provider_configs
				WHEN NEW.plugin_id='io.mender.provider.workspace' AND NEW.plugin_installation_id IS NULL AND NEW.config_version!=2
				BEGIN SELECT RAISE(ABORT,'Workspace Provider v1 retired'); END`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("retire Provider compatibility schema: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO provider_config_schema_migrations(version,applied_at) VALUES(4,?)",
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record Provider hard-cut migration 4: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider config migrations: %w", err)
	}
	return nil
}
