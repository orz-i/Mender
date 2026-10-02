package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func migrate(
	ctx context.Context,
	db *sql.DB,
	defaultIsolation runtimegovernancedomain.Isolation,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin runtime governance migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runtime_governance_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create runtime governance migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM runtime_governance_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check runtime governance schema version: %w", err)
	}
	const latestSupportedVersion = 2
	if recordedVersion.Valid && recordedVersion.Int64 > latestSupportedVersion {
		return fmt.Errorf(
			"runtime governance schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSupportedVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM runtime_governance_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check runtime governance migration 1: %w", err)
	}
	if applied == 0 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE runtime_budget_settings (
			singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
			max_invocations INTEGER NOT NULL,
			max_principal_invocations INTEGER NOT NULL,
			max_provider_invocations INTEGER NOT NULL,
			max_tool_invocations INTEGER NOT NULL,
			max_provider_activations INTEGER NOT NULL,
			max_execution_timeout_ms INTEGER NOT NULL,
			max_argument_bytes INTEGER NOT NULL,
			max_result_bytes INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("apply runtime governance migration 1: %w", err)
		}
		spec := runtimegovernancedomain.DefaultBudget().Spec()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO runtime_budget_settings(
				singleton_id,
				max_invocations,
				max_principal_invocations,
				max_provider_invocations,
				max_tool_invocations,
				max_provider_activations,
				max_execution_timeout_ms,
				max_argument_bytes,
				max_result_bytes,
				updated_at
			) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			spec.MaxInvocations,
			spec.MaxPrincipalInvocations,
			spec.MaxProviderInvocations,
			spec.MaxToolInvocations,
			spec.MaxProviderActivations,
			spec.MaxExecutionTimeoutMS,
			spec.MaxArgumentBytes,
			spec.MaxResultBytes,
			now,
		); err != nil {
			return fmt.Errorf("initialize runtime budget: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO runtime_governance_schema_migrations(version, applied_at) VALUES (1, ?)",
			now,
		); err != nil {
			return fmt.Errorf("record runtime governance migration 1: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM runtime_governance_schema_migrations WHERE version = 2",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check runtime governance migration 2: %w", err)
	}
	if applied == 0 {
		if err := defaultIsolation.Validate(); err != nil {
			return fmt.Errorf("runtime governance migration 2 default isolation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `CREATE TABLE runtime_isolation_settings (
			singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
			workspace_shell TEXT NOT NULL,
			external_provider TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("apply runtime governance migration 2: %w", err)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO runtime_isolation_settings(singleton_id, workspace_shell, external_provider, updated_at) VALUES (1, ?, ?, ?)",
			string(defaultIsolation.WorkspaceShell()),
			string(defaultIsolation.ExternalProvider()),
			now,
		); err != nil {
			return fmt.Errorf("initialize runtime isolation: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO runtime_governance_schema_migrations(version, applied_at) VALUES (2, ?)",
			now,
		); err != nil {
			return fmt.Errorf("record runtime governance migration 2: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit runtime governance migrations: %w", err)
	}
	return nil
}
