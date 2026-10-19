package sqlite

import (
	"context"
	"database/sql"
	"testing"
)

// This helper is test-only: synthetic downgrade fixtures must remove the
// v18 Task column and marker before reopening as an older schema.
func stripTaskBudgetV18Fixture(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		"ALTER TABLE agent_tasks DROP COLUMN timeout_seconds",
		"DELETE FROM agent_schema_migrations WHERE version=18",
	} {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("strip synthetic v18 Task budget %q: %v", statement, err)
		}
	}
}

// Legacy migration fixtures intentionally emulate a database before v14.
// Remove the newly created evidence-only objects before downgrading their
// older agent tables; never do this to a production database.
func stripExecutionEvidenceV14Fixture(t *testing.T, db *sql.DB) {
	t.Helper()
	stripTaskBudgetV18Fixture(t, db)
	for _, statement := range []string{
		"DROP TRIGGER IF EXISTS agent_continuation_no_update",
		"DROP TRIGGER IF EXISTS agent_continuation_no_delete",
		"DROP TABLE IF EXISTS agent_task_continuations",
		"DELETE FROM agent_schema_migrations WHERE version=17",
		"DELETE FROM agent_schema_migrations WHERE version=15",
		"DROP TRIGGER IF EXISTS agent_execution_event_no_delete",
		"DROP TRIGGER IF EXISTS agent_execution_event_no_update",
		"DROP TRIGGER IF EXISTS agent_native_session_no_delete",
		"DROP TRIGGER IF EXISTS agent_native_session_no_update",
		"DROP TABLE IF EXISTS agent_execution_events",
		"DROP TABLE IF EXISTS agent_execution_processes",
		"DROP TABLE IF EXISTS agent_execution_attempts",
		"DROP TABLE IF EXISTS agent_native_sessions",
		"DELETE FROM agent_schema_migrations WHERE version=14",
	} {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("strip synthetic v14 evidence %q: %v", statement, err)
		}
	}
}
