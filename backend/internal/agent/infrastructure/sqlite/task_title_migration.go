package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// v19 adds only an explicitly supplied Task display title. Historical Tasks
// remain unnamed; neither prompt nor inferred summaries enter persistence.
func migrateAgentTaskTitleV19(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version >= 19 {
		return tx.Commit()
	}
	if version != 18 {
		return fmt.Errorf("agent v19 Task title requires v18, got %d", version)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE agent_tasks ADD COLUMN title TEXT NOT NULL DEFAULT ''
		CHECK(length(title) <= 80)`); err != nil {
		return fmt.Errorf("agent v19 Task title schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations(version,applied_at) VALUES(19,?)",
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("agent v19 Task title migration record: %w", err)
	}
	return tx.Commit()
}
