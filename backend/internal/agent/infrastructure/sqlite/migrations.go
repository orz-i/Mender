package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 18

// migrate owns Agent state exclusively, and never changes the existing
// Providers/Sandboxes/Tunnels/Plugin inventory.
func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS agent_schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	var version sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT MAX(version) FROM agent_schema_migrations`).Scan(&version); err != nil {
		return err
	}
	if version.Valid && version.Int64 > latestSchemaVersion {
		return fmt.Errorf("agent schema %d is newer than runtime %d", version.Int64, latestSchemaVersion)
	}
	if !version.Valid {
		for _, statement := range []string{
			`CREATE TABLE agents (
				agent_id TEXT PRIMARY KEY, name TEXT NOT NULL, backend TEXT NOT NULL,
				plugin_id TEXT NOT NULL, extension_id TEXT NOT NULL,
				workspace_root TEXT NOT NULL, isolation TEXT NOT NULL,
				owner_kind TEXT NOT NULL, owner_ref TEXT NOT NULL,
				created_at TEXT NOT NULL,
				CHECK(backend IN ('codex','grok')), CHECK(isolation IN ('host','restricted_host'))
			)`,
			`CREATE TABLE agent_tasks (
				task_id TEXT PRIMARY KEY, agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE RESTRICT,
				status TEXT NOT NULL, origin TEXT NOT NULL,
				principal_kind TEXT NOT NULL, principal_ref TEXT NOT NULL,
				scope_kind TEXT NOT NULL, scope_ref TEXT NOT NULL,
				generation INTEGER NOT NULL, result_text TEXT NOT NULL DEFAULT '',
				diagnostic TEXT NOT NULL DEFAULT '', cancel_requested INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
				CHECK(status IN ('queued','running','completed','failed','cancelled','unknown')),
				CHECK(length(result_text)<=65536), CHECK(length(diagnostic)<=1024)
			)`,
			`CREATE INDEX agent_tasks_owner ON agent_tasks(principal_kind,principal_ref,scope_kind,scope_ref,created_at)`,
			`CREATE INDEX agent_tasks_by_agent ON agent_tasks(agent_id,created_at)`,
			`CREATE TABLE agent_grants (
				agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
				profile_id TEXT NOT NULL, created_at TEXT NOT NULL,
				PRIMARY KEY(agent_id,profile_id)
			)`,
			`CREATE TRIGGER agent_task_terminal_immutable BEFORE UPDATE ON agent_tasks
				WHEN OLD.status IN ('completed','failed','cancelled','unknown')
				BEGIN SELECT RAISE(ABORT,'terminal Agent Task immutable'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("agent schema: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`, 1, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if !version.Valid || version.Int64 < 2 {
		// Grant/revoke history survives Agent deletion. SQL triggers record
		// changes even if a caller bypasses application-level convenience
		// APIs; records contain no task prompt, API key or result payload.
		for _, statement := range []string{
			`CREATE TABLE agent_grant_events (
				event_id INTEGER PRIMARY KEY AUTOINCREMENT,
				agent_id TEXT NOT NULL,
				profile_id TEXT NOT NULL,
				action TEXT NOT NULL CHECK(action IN ('grant','revoke')),
				occurred_at TEXT NOT NULL
			)`,
			`CREATE INDEX agent_grant_events_agent ON agent_grant_events(agent_id,event_id)`,
			`CREATE TRIGGER agent_grant_insert_event AFTER INSERT ON agent_grants
				BEGIN INSERT INTO agent_grant_events(agent_id,profile_id,action,occurred_at)
					VALUES(NEW.agent_id,NEW.profile_id,'grant',STRFTIME('%Y-%m-%dT%H:%M:%fZ','now')); END`,
			`CREATE TRIGGER agent_grant_delete_event AFTER DELETE ON agent_grants
				BEGIN INSERT INTO agent_grant_events(agent_id,profile_id,action,occurred_at)
					VALUES(OLD.agent_id,OLD.profile_id,'revoke',STRFTIME('%Y-%m-%dT%H:%M:%fZ','now')); END`,
			`CREATE TRIGGER agent_grant_history_no_update BEFORE UPDATE ON agent_grant_events
				BEGIN SELECT RAISE(ABORT,'Agent grant history immutable'); END`,
			`CREATE TRIGGER agent_grant_history_no_delete BEFORE DELETE ON agent_grant_events
				BEGIN SELECT RAISE(ABORT,'Agent grant history immutable'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("agent v2 grant history: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`,
			2, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if !version.Valid || version.Int64 < 3 {
		for _, statement := range []string{
			`CREATE TABLE agent_admissions (
				event_id INTEGER PRIMARY KEY AUTOINCREMENT,
				agent_id TEXT NOT NULL,
				action TEXT NOT NULL CHECK(action IN ('agent.activate','agent.task.submit','agent.task.cancel')),
				principal_kind TEXT NOT NULL,principal_ref TEXT NOT NULL,
				origin TEXT NOT NULL,scope_kind TEXT NOT NULL,scope_ref TEXT NOT NULL,
				policy_kind TEXT NOT NULL CHECK(policy_kind IN ('allow','deny','require_approval','failed')),
				policy_reason TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('admitted','denied','failed')),
				occurred_at TEXT NOT NULL
			)`,
			`CREATE INDEX agent_admissions_by_agent ON agent_admissions(agent_id,event_id)`,
			`CREATE TRIGGER agent_admission_no_update BEFORE UPDATE ON agent_admissions
				BEGIN SELECT RAISE(ABORT,'Agent admission audit immutable'); END`,
			`CREATE TRIGGER agent_admission_no_delete BEFORE DELETE ON agent_admissions
				BEGIN SELECT RAISE(ABORT,'Agent admission audit immutable'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("agent v3 admission audit: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`,
			3, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	// Agent -> Provider references are embedded in a typed Provider config
	// JSON field. This trigger prevents a concurrent delete/rebind from
	// leaving dangling Providers, at SQLite commit rather than in a racy
	// management read. Some standalone Agent-store tests intentionally have
	// no Provider catalogue; the trigger is installed on Runtime composition.
	var providerConfigs int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='provider_configs'`).Scan(&providerConfigs); err != nil {
		return err
	}
	if providerConfigs > 0 {
		if _, err = tx.ExecContext(ctx, `CREATE TRIGGER IF NOT EXISTS agent_bound_provider_delete_guard
			BEFORE DELETE ON agents
			WHEN EXISTS(
				SELECT 1 FROM provider_configs p
				WHERE p.plugin_id='io.mender.provider.agent-delegation'
				  AND (json_valid(p.config_json)=0 OR
					   json_extract(p.config_json,'$.agentId')=OLD.agent_id)
			)
			BEGIN SELECT RAISE(ABORT,'Agent is referenced by a Provider'); END`); err != nil {
			return fmt.Errorf("agent Provider binding guard: %w", err)
		}
	}
	if !version.Valid || version.Int64 < 4 {
		// Managed suite bindings are host-owned. Existing manual Providers are
		// not adopted on startup: only an explicit owner mutation can claim one.
		for _, statement := range []string{
			`CREATE TABLE agent_tool_facades (
				agent_id TEXT PRIMARY KEY REFERENCES agents(agent_id) ON DELETE RESTRICT,
				provider_id TEXT NOT NULL UNIQUE REFERENCES providers(id) ON DELETE RESTRICT,
				suite_id TEXT NOT NULL CHECK(suite_id='agent.delegation'),
				created_at TEXT NOT NULL
			)`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("agent v4 tool facades: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`,
			4, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	// V5 hardens the exact managed facade contract. The old trigger omitted
	// config_version, permitting a claimed Provider to be retargeted to a
	// different protocol schema without changing its AgentID. Recreate that
	// trigger in the same forward-only schema transaction.
	if !version.Valid || version.Int64 < 5 {
		if _, err = tx.ExecContext(ctx, `DROP TRIGGER IF EXISTS agent_facade_provider_config_update_guard`); err != nil {
			return fmt.Errorf("agent v5 drop legacy facade guard: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`,
			5, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("agent v5 migration record: %w", err)
		}
	}
	if !version.Valid || version.Int64 < 6 {
		// Keep live Agent and Tool identities. Runtime selections are snapshot
		// fields, and additional local targets remain explicit owner bindings.
		for _, statement := range []string{
			`ALTER TABLE agents ADD COLUMN default_model TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE agent_tasks ADD COLUMN model TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE agent_tasks ADD COLUMN workspace_key TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE agent_tasks ADD COLUMN workspace_root TEXT NOT NULL DEFAULT ''`,
			`CREATE TABLE agent_workspaces (
				agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
				workspace_key TEXT NOT NULL, sandbox_id TEXT NOT NULL,
				created_at TEXT NOT NULL, PRIMARY KEY(agent_id,workspace_key),
				UNIQUE(agent_id,sandbox_id))`,
			`CREATE TABLE agent_workspace_grants (
				agent_id TEXT NOT NULL,workspace_key TEXT NOT NULL,profile_id TEXT NOT NULL,
				created_at TEXT NOT NULL,
				PRIMARY KEY(agent_id,workspace_key,profile_id),
				FOREIGN KEY(agent_id,workspace_key) REFERENCES agent_workspaces(agent_id,workspace_key) ON DELETE CASCADE)`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("agent v6 runtime selections: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`, 6, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if !version.Valid || version.Int64 < 7 {
		// Existing Agent and Task identities remain unchanged. Empty effort
		// means the agent backend default and never rewrites old snapshots.
		for _, statement := range []string{
			`ALTER TABLE agents ADD COLUMN default_reasoning_effort TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE agent_tasks ADD COLUMN reasoning_effort TEXT NOT NULL DEFAULT ''`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("agent v7 reasoning effort: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`, 7, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	// Standalone Agent stores may not yet have a Provider catalogue. The
	// guards are installed whenever the Runtime composes both domains.
	if providerConfigs > 0 {
		for _, statement := range []string{
			`CREATE TRIGGER IF NOT EXISTS agent_facade_provider_config_delete_guard BEFORE DELETE ON provider_configs
				WHEN EXISTS(SELECT 1 FROM agent_tool_facades f WHERE f.provider_id=OLD.provider_id)
				BEGIN SELECT RAISE(ABORT,'managed Agent facade must be detached by owner'); END`,
			`CREATE TRIGGER IF NOT EXISTS agent_facade_provider_config_update_guard BEFORE UPDATE OF plugin_id, config_json, config_version, plugin_installation_id ON provider_configs
				WHEN EXISTS(SELECT 1 FROM agent_tool_facades f WHERE f.provider_id=OLD.provider_id)
				BEGIN SELECT RAISE(ABORT,'managed Agent facade config is immutable'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("agent tool facade guards: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := migrateAgentBackendV8(ctx, db); err != nil {
		return err
	}
	if err := migrateAgentWorkspaceV9(ctx, db); err != nil {
		return err
	}
	if err := migrateApprovalsV10(ctx, db); err != nil {
		return err
	}
	if err := migrateToolPlacementV11(ctx, db); err != nil {
		return err
	}
	if err := migrateApprovalPolicyV12(ctx, db); err != nil {
		return err
	}
	if err := migrateAgentBackendV13(ctx, db); err != nil {
		return err
	}
	if err := migrateExecutionEvidenceV14(ctx, db); err != nil {
		return err
	}
	if err := migrateNativeResumeV15(ctx, db); err != nil {
		return err
	}
	if err := migrateRetireUnknownDispositionV17(ctx, db); err != nil {
		return err
	}
	return migrateAgentTaskBudgetV18(ctx, db)
}
