package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 3

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
	return tx.Commit()
}
