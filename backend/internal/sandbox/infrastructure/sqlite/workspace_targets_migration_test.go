package sqlite

import (
	"context"
	"database/sql"
	"testing"
)

// rewindWorkspaceTargetsV19Fixture synthesizes historical Sandbox schemas in
// ephemeral test databases only. A new v19 table/row must not survive a
// deliberate downgrade to v8–v17, or the forward migrator cannot be exercised.
func rewindWorkspaceTargetsV19Fixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	var targets int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_tool_targets").Scan(&targets); err != nil {
		t.Fatal(err)
	}
	if targets != 0 {
		t.Fatal("historical fixture must not erase active workspace targets")
	}
	for _, query := range []string{
		"DROP TRIGGER IF EXISTS workspace_tool_target_suite_guard",
		"DROP TRIGGER IF EXISTS workspace_tool_target_sandbox_guard",
		"DROP TABLE IF EXISTS workspace_tool_targets",
		"DELETE FROM sandbox_schema_migrations WHERE version=19",
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatalf("rewind v19 fixture %q: %v", query, err)
		}
	}
}
