package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// v10 adds only an owner-facing bounded approval ledger. Task, Agent,
// Provider, Tool and Profile identities remain unchanged. Every pending
// approval from a prior process is invalidated on runtime startup.
func migrateApprovalsV10(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version >= 10 {
		return tx.Commit()
	}
	if version != 9 {
		return fmt.Errorf("agent v10 approvals require v9; got %d", version)
	}
	for _, statement := range []string{
		`CREATE TABLE agent_task_approvals (
   approval_id TEXT PRIMARY KEY,
   task_id TEXT NOT NULL REFERENCES agent_tasks(task_id) ON DELETE RESTRICT,
   agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE RESTRICT,
   generation INTEGER NOT NULL CHECK(generation>0),
   session_id TEXT NOT NULL,
   tool_call_id TEXT NOT NULL,
   action_kind TEXT NOT NULL CHECK(action_kind IN ('read','edit','execute','other')),
   allow_option_id TEXT NOT NULL, reject_option_id TEXT NOT NULL,
   status TEXT NOT NULL CHECK(status IN ('pending','allowed_once','rejected','expired')),
   created_at TEXT NOT NULL,
   expires_unix_ms INTEGER NOT NULL,
   resolved_at TEXT NOT NULL DEFAULT '',
   UNIQUE(task_id,generation,session_id,tool_call_id)
  )`,
		`CREATE INDEX agent_approvals_by_task ON agent_task_approvals(task_id,status,created_at)`,
		`CREATE INDEX agent_approvals_by_agent ON agent_task_approvals(agent_id,created_at)`,
		// Resolved approvals are terminal audit evidence. Only pending -> one
		// terminal decision is possible; no delete/rewriting completed approvals.
		`CREATE TRIGGER agent_approval_terminal_no_update BEFORE UPDATE ON agent_task_approvals
    WHEN OLD.status!='pending' BEGIN SELECT RAISE(ABORT,'resolved Agent approval immutable'); END`,
		`CREATE TRIGGER agent_approval_no_delete BEFORE DELETE ON agent_task_approvals
    BEGIN SELECT RAISE(ABORT,'Agent approval history immutable'); END`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("agent v10 approval schema: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations VALUES(?,?)", 10, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
