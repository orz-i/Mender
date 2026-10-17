package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func migrateToolPlacementV11(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version >= 11 {
		return tx.Commit()
	}
	if version != 10 {
		return fmt.Errorf("agent v11 placement migration requires v10, got %d", version)
	}
	for _, sqlText := range []string{
		`CREATE TABLE agent_tool_placements (
    agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    workspace_key TEXT NOT NULL,
    workspace_root TEXT NOT NULL,
    sandbox_id TEXT NOT NULL,
    tool_ids_json TEXT NOT NULL CHECK(json_valid(tool_ids_json)),
    updated_at TEXT NOT NULL,
    PRIMARY KEY(agent_id,workspace_key)
  )`,
		`CREATE TRIGGER agent_placement_registered_workspace_delete_guard
    BEFORE DELETE ON agent_workspaces
    WHEN EXISTS (SELECT 1 FROM agent_tool_placements p
      WHERE p.agent_id=OLD.agent_id AND p.workspace_key=OLD.workspace_key)
    BEGIN SELECT RAISE(ABORT,'disable Agent governed tools before removing workspace'); END`,
	} {
		if _, err := tx.ExecContext(ctx, sqlText); err != nil {
			return fmt.Errorf("agent v11 placement schema: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations VALUES(?,?)", 11, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
