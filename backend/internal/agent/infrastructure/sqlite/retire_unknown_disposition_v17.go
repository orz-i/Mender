package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// v17 permanently retires the v16 unknown-task handling experiment. This is
// a forward-only hard cut, not a compatibility layer: the only supported
// runtime model is the v15 AgentTask outcome and native Resume contract.
// A database previously started on v16 is cleaned transactionally; one
// previously on v15 advances directly without ever creating v16 tables.
func migrateRetireUnknownDispositionV17(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version >= 17 {
		return tx.Commit()
	}
	if version != 15 && version != 16 {
		return fmt.Errorf("agent v17 retirement requires v15 or v16, got %d", version)
	}

	if version == 16 {
		// Reject unknown schema objects rather than implicitly destroying
		// unrelated safety triggers when dropping the retired table.
		expected := map[string]string{
			"agent_unknown_dispositions":          "table",
			"agent_unknown_dispositions_by_task":  "index",
			"agent_unknown_disposition_no_update": "trigger",
			"agent_unknown_disposition_no_delete": "trigger",
		}
		rows, err := tx.QueryContext(ctx, `SELECT type,name FROM sqlite_master
			WHERE tbl_name='agent_unknown_dispositions'
				AND type IN ('table','index','trigger')`)
		if err != nil {
			return err
		}
		found := make(map[string]bool, len(expected))
		for rows.Next() {
			var kind, name string
			if err := rows.Scan(&kind, &name); err != nil {
				_ = rows.Close()
				return err
			}
			if expected[name] != kind || found[name] {
				_ = rows.Close()
				return fmt.Errorf("agent v17 refuses unreviewed retired schema object")
			}
			found[name] = true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(found) != len(expected) {
			return fmt.Errorf("agent v17 retired schema is incomplete")
		}
		for _, stmt := range []string{
			"DROP TRIGGER agent_unknown_disposition_no_update",
			"DROP TRIGGER agent_unknown_disposition_no_delete",
			"DROP INDEX agent_unknown_dispositions_by_task",
			"DROP TABLE agent_unknown_dispositions",
			"DELETE FROM agent_schema_migrations WHERE version=16",
		} {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("agent v17 retire unknown disposition: %w", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO agent_schema_migrations(version,applied_at) VALUES(17,?)",
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
