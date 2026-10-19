package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// v18 snapshots the execution budget for each Task. Historical Tasks keep
// their original ten-minute deadline. No Task result or grant is rewritten.
// The new Task's budget is set by an admitted, bounded submission request;
// it cannot be extended while work is already running.
func migrateAgentTaskBudgetV18(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version >= 18 {
		return tx.Commit()
	}
	if version != 17 {
		return fmt.Errorf("agent v18 Task budget requires v17, got %d", version)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE agent_tasks
		ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 600
		CHECK(timeout_seconds IN (600,1200,1800))`); err != nil {
		return fmt.Errorf("agent v18 Task budget schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations(version,applied_at) VALUES(18,?)",
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("agent v18 Task budget migration record: %w", err)
	}
	return tx.Commit()
}
