package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 1

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
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`, latestSchemaVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
