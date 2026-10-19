package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// v15 binds explicit continuation Tasks to an immutable native Session and
// records separate resume admissions without broadening existing grants.
func migrateNativeResumeV15(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version >= 15 {
		return tx.Commit()
	}
	if version != 14 {
		return fmt.Errorf("agent v15 native resume requires v14, got %d", version)
	}
	// Rebuilding the immutable admission ledger cannot silently destroy
	// an unreviewed trigger or index that another Runtime depends on.
	for _, query := range []string{
		`SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name='agent_admissions'`,
		`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='agent_admissions'`,
	} {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err != nil {
				_ = rows.Close()
				return err
			}
			if name != "agent_admission_no_update" && name != "agent_admission_no_delete" &&
				name != "agent_admissions_by_agent" {
				_ = rows.Close()
				return fmt.Errorf("agent v15 refuses unreviewed admission schema object")
			}
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`CREATE TABLE agent_task_continuations (
           child_task_id TEXT PRIMARY KEY REFERENCES agent_tasks(task_id) ON DELETE RESTRICT,
           parent_task_id TEXT NOT NULL REFERENCES agent_tasks(task_id) ON DELETE RESTRICT,
           session_ref TEXT NOT NULL REFERENCES agent_native_sessions(session_ref) ON DELETE RESTRICT,
           created_at TEXT NOT NULL
        )`,
		`CREATE INDEX agent_continuations_by_session ON agent_task_continuations(session_ref,child_task_id)`,
		`CREATE INDEX agent_continuations_by_parent ON agent_task_continuations(parent_task_id,child_task_id)`,
		`CREATE TRIGGER agent_continuation_no_update BEFORE UPDATE ON agent_task_continuations
          BEGIN SELECT RAISE(ABORT,'Agent Task continuation immutable'); END`,
		`CREATE TRIGGER agent_continuation_no_delete BEFORE DELETE ON agent_task_continuations
          BEGIN SELECT RAISE(ABORT,'Agent Task continuation immutable'); END`,
		// Rebuild the narrow admission CHECK without removing any old evidence.
		`CREATE TABLE agent_admissions_v15 (
           event_id INTEGER PRIMARY KEY AUTOINCREMENT,
           agent_id TEXT NOT NULL,
           action TEXT NOT NULL CHECK(action IN ('agent.activate','agent.task.submit','agent.task.resume','agent.task.cancel')),
           principal_kind TEXT NOT NULL,principal_ref TEXT NOT NULL,
           origin TEXT NOT NULL,scope_kind TEXT NOT NULL,scope_ref TEXT NOT NULL,
           policy_kind TEXT NOT NULL CHECK(policy_kind IN ('allow','deny','require_approval','failed')),
           policy_reason TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('admitted','denied','failed')),
           occurred_at TEXT NOT NULL
        )`,
		`INSERT INTO agent_admissions_v15 SELECT * FROM agent_admissions`,
		`DROP TABLE agent_admissions`,
		`ALTER TABLE agent_admissions_v15 RENAME TO agent_admissions`,
		`CREATE INDEX agent_admissions_by_agent ON agent_admissions(agent_id,event_id)`,
		`CREATE TRIGGER agent_admission_no_update BEFORE UPDATE ON agent_admissions
           BEGIN SELECT RAISE(ABORT,'Agent admission audit immutable'); END`,
		`CREATE TRIGGER agent_admission_no_delete BEFORE DELETE ON agent_admissions
           BEGIN SELECT RAISE(ABORT,'Agent admission audit immutable'); END`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("agent native resume v15: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations VALUES(?,?)",
		15, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
