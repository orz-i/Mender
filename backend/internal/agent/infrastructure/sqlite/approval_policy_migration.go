package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// v12 adds explicit owner-selected per-Agent/workspace ACP permission modes.
// No previous Host Agent is granted any permission by migration.
func migrateApprovalPolicyV12(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version >= 12 {
		return tx.Commit()
	}
	if version != 11 {
		return fmt.Errorf("agent v12 approval policy requires v11, got %d", version)
	}
	for _, stmt := range []string{
		`CREATE TABLE agent_approval_policies (
   agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
   workspace_key TEXT NOT NULL,
   workspace_root TEXT NOT NULL,
   mode TEXT NOT NULL CHECK(mode IN ('workspace_read','trusted')),
   expires_unix_ms INTEGER NOT NULL DEFAULT 0 CHECK(expires_unix_ms>=0),
   updated_at TEXT NOT NULL,
   PRIMARY KEY(agent_id,workspace_key)
  )`,
		`ALTER TABLE agent_task_approvals ADD COLUMN decision_source TEXT NOT NULL DEFAULT 'owner' CHECK(decision_source IN ('owner','policy'))`,
		`CREATE TRIGGER agent_workspace_approval_policy_unbind BEFORE DELETE ON agent_workspaces
    WHEN EXISTS (SELECT 1 FROM agent_approval_policies p
       WHERE p.agent_id=OLD.agent_id AND p.workspace_key=OLD.workspace_key)
    BEGIN SELECT RAISE(ABORT,'disable Agent workspace approval mode before unbinding'); END`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("agent v12 approval policy: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations VALUES(?,?)", 12, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
