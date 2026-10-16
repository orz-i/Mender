package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// migrateAgentBackendV8 extends the *existing* Agent inventory to native
// OpenCode. SQLite cannot DROP/REPLACE a CHECK in place. Rebuild exactly the
// Agent table on a pinned single connection with FK enforcement temporarily
// disabled, validate references BEFORE COMMIT, and restore FK enforcement
// before this connection returns to its pool. No user/Tool identity changes.
func migrateAgentBackendV8(ctx context.Context, db *sql.DB) (retErr error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, conn.Close()) }()
	var version int
	if err := conn.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version == 8 {
		return nil
	}
	if version != 7 {
		return fmt.Errorf("agent v8 migration requires v7, found %d", version)
	}

	// A future migration must explicitly review any new Agent-table triggers
	// or indices instead of silently discarding them in an old rebuild.
	rows, err := conn.QueryContext(ctx, `SELECT type,name FROM sqlite_master
 WHERE tbl_name='agents' AND ((type='trigger') OR (type='index' AND sql IS NOT NULL))`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			_ = rows.Close()
			return err
		}
		if kind != "trigger" || name != "agent_bound_provider_delete_guard" {
			_ = rows.Close()
			return fmt.Errorf("agent v8 refuses unexpected %s on agents: %s", kind, name)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if err := assertAgentForeignKeys(ctx, conn); err != nil {
		return fmt.Errorf("agent v8 preflight: %w", err)
	}
	var enabled int
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return err
	}
	if enabled != 1 {
		return fmt.Errorf("agent v8 requires foreign key enforcement before migration")
	}

	// Changing PRAGMA foreign_keys inside a transaction is silently ignored,
	// hence this preflight runs on the exclusively pinned connection.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(restoreCtx, "PRAGMA foreign_keys=ON"); e != nil {
			retErr = errors.Join(retErr, fmt.Errorf("restore Agent FK enforcement: %w", e))
		} else {
			var fk int
			if e := conn.QueryRowContext(restoreCtx, "PRAGMA foreign_keys").Scan(&fk); e != nil {
				retErr = errors.Join(retErr, fmt.Errorf("verify restored Agent FK enforcement: %w", e))
			} else if fk != 1 {
				retErr = errors.Join(retErr, fmt.Errorf("agent FK enforcement failed to restore: state=%d", fk))
			}
		}
	}()
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return fmt.Errorf("read Agent FK state for rebuild: %w", err)
	}
	if enabled != 0 {
		return fmt.Errorf("agent v8 cannot disable FK during table rebuild")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// The canonical table structure is otherwise identical to v7.
	// Use explicit fields and preserve original defaults and IDs verbatim.
	for _, sqlText := range []string{
		`CREATE TABLE agents_v8 (
    agent_id TEXT PRIMARY KEY, name TEXT NOT NULL, backend TEXT NOT NULL,
    plugin_id TEXT NOT NULL, extension_id TEXT NOT NULL,
    workspace_root TEXT NOT NULL, isolation TEXT NOT NULL,
    owner_kind TEXT NOT NULL, owner_ref TEXT NOT NULL, created_at TEXT NOT NULL,
    default_model TEXT NOT NULL DEFAULT '',
    default_reasoning_effort TEXT NOT NULL DEFAULT '',
    CHECK(backend IN ('codex','grok','opencode')),
    CHECK(isolation IN ('host','restricted_host'))
   )`,
		`INSERT INTO agents_v8(agent_id,name,backend,plugin_id,extension_id,workspace_root,
    isolation,owner_kind,owner_ref,created_at,default_model,default_reasoning_effort)
   SELECT agent_id,name,backend,plugin_id,extension_id,workspace_root,
    isolation,owner_kind,owner_ref,created_at,default_model,default_reasoning_effort
   FROM agents`,
		`DROP TABLE agents`,
		`ALTER TABLE agents_v8 RENAME TO agents`,
	} {
		if _, err := tx.ExecContext(ctx, sqlText); err != nil {
			return fmt.Errorf("agent v8 rebuild: %w", err)
		}
	}
	var providerTable int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
 WHERE name='provider_configs' AND type='table'`).Scan(&providerTable); err != nil {
		return err
	}
	if providerTable > 0 {
		if _, err := tx.ExecContext(ctx, `CREATE TRIGGER agent_bound_provider_delete_guard
   BEFORE DELETE ON agents
   WHEN EXISTS(
    SELECT 1 FROM provider_configs p
    WHERE p.plugin_id='io.mender.provider.agent-delegation'
      AND (json_valid(p.config_json)=0 OR json_extract(p.config_json,'$.agentId')=OLD.agent_id)
   )
   BEGIN SELECT RAISE(ABORT,'Agent is referenced by a Provider'); END`); err != nil {
			return fmt.Errorf("agent v8 restore Provider deletion guard: %w", err)
		}
	}
	if err := assertAgentForeignKeys(ctx, tx); err != nil {
		return fmt.Errorf("agent v8 post-copy FK integrity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_schema_migrations VALUES(?,?)`,
		8, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent v8 backend constraint: %w", err)
	}
	return nil
}

type fkQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func assertAgentForeignKeys(ctx context.Context, db fkQuerier) error {
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var table, tableID string
		var rowID sql.NullInt64
		var fkIndex int
		if err := rows.Scan(&table, &rowID, &tableID, &fkIndex); err != nil {
			return err
		}
		return fmt.Errorf("foreign key check failed in table %s", table)
	}
	return rows.Err()
}
