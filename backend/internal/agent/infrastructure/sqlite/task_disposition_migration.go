package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// v16 stores owner-reported work disposition separately from the AgentTask
// outcome. No existing unknown Task is annotated automatically on migration.
func migrateTaskDispositionV16(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version >= 16 {
		return tx.Commit()
	}
	if version != 15 {
		return fmt.Errorf("agent v16 disposition requires v15, got %d", version)
	}
	for _, stmt := range []string{
		`CREATE TABLE agent_unknown_dispositions (
			event_id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL REFERENCES agent_tasks(task_id) ON DELETE RESTRICT,
			handling TEXT NOT NULL CHECK(handling IN ('handled_elsewhere','needs_review')),
			occurred_at TEXT NOT NULL
		)`,
		`CREATE INDEX agent_unknown_dispositions_by_task ON agent_unknown_dispositions(task_id,event_id)`,
		`CREATE TRIGGER agent_unknown_disposition_no_update BEFORE UPDATE ON agent_unknown_dispositions
			BEGIN SELECT RAISE(ABORT,'Agent unknown disposition history immutable'); END`,
		`CREATE TRIGGER agent_unknown_disposition_no_delete BEFORE DELETE ON agent_unknown_dispositions
			BEGIN SELECT RAISE(ABORT,'Agent unknown disposition history immutable'); END`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("agent v16 disposition: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations VALUES(?,?)",
		16, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
