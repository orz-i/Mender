package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// migrateAgentWorkspaceV9 hard-cuts Agent cwd aliases from Sandbox references
// to canonical owner-approved local paths. It changes no Sandbox or Tool data.
// All historical rows and Profile grants survive, or the entire transaction
// fails without changing the schema.
func migrateAgentWorkspaceV9(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version == 9 {
		return nil
	}
	if version != 8 {
		return fmt.Errorf("agent v9 workspace migration requires v8, found %d", version)
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_workspaces").Scan(&count); err != nil {
		return err
	}

	type legacyBinding struct{ agentID, key, sandboxID, root string }
	legacy := make([]legacyBinding, 0, count)
	if count > 0 {
		var sandboxTables int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
   WHERE type='table' AND name IN ('sandboxes','sandbox_runs')`).Scan(&sandboxTables); err != nil {
			return err
		}
		if sandboxTables != 2 {
			return fmt.Errorf("agent v9 refuses to migrate linked workspaces without Sandbox inventory")
		}

		rows, err := tx.QueryContext(ctx, `SELECT w.agent_id,w.workspace_key,w.sandbox_id,
   a.isolation,a.owner_kind,a.owner_ref,
   s.backend,s.isolation,s.local_root,s.owner_kind,s.owner_ref,r.state
   FROM agent_workspaces w JOIN agents a ON a.agent_id=w.agent_id
   LEFT JOIN sandboxes s ON s.id=w.sandbox_id
   LEFT JOIN sandbox_runs r ON r.sandbox_id=s.id
   ORDER BY w.agent_id,w.workspace_key`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var agentID, key, sandboxID, agentIsolation, ownerKind, ownerRef string
			var backend, sandboxIsolation, root, sandboxOwnerKind, sandboxOwnerRef, state sql.NullString
			if err := rows.Scan(&agentID, &key, &sandboxID, &agentIsolation, &ownerKind, &ownerRef,
				&backend, &sandboxIsolation, &root, &sandboxOwnerKind, &sandboxOwnerRef, &state); err != nil {
				_ = rows.Close()
				return err
			}
			if !backend.Valid || backend.String != "local_workspace" || !state.Valid || state.String != "ready" ||
				!sandboxIsolation.Valid || (sandboxIsolation.String != "host" && sandboxIsolation.String != "restricted_host") ||
				(agentIsolation == "host" && sandboxIsolation.String == "restricted_host") ||
				!sandboxOwnerKind.Valid || sandboxOwnerKind.String != ownerKind ||
				!sandboxOwnerRef.Valid || sandboxOwnerRef.String != ownerRef ||
				!root.Valid || root.String == "" || !filepath.IsAbs(root.String) || filepath.Clean(root.String) != root.String {
				_ = rows.Close()
				return fmt.Errorf("agent v9 migration blocked by invalid/unavailable local Sandbox for workspace %q", key)
			}
			canonical, err := filepath.EvalSymlinks(root.String)
			if err != nil || canonical != root.String {
				_ = rows.Close()
				return fmt.Errorf("agent v9 migration blocked by noncanonical local directory for workspace %q", key)
			}
			info, err := os.Stat(canonical)
			if err != nil || !info.IsDir() {
				_ = rows.Close()
				return fmt.Errorf("agent v9 migration blocked by missing local directory for workspace %q", key)
			}
			legacy = append(legacy, legacyBinding{agentID: agentID, key: key, sandboxID: sandboxID, root: canonical})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(legacy) != count {
			return errors.New("agent v9 migration cannot account for every legacy workspace")
		}
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE agent_workspaces RENAME COLUMN sandbox_id TO workspace_root"); err != nil {
		return fmt.Errorf("agent v9 hard-cut local workspace schema: %w", err)
	}
	for _, link := range legacy {
		res, err := tx.ExecContext(ctx, `UPDATE agent_workspaces SET workspace_root=?
   WHERE agent_id=? AND workspace_key=? AND workspace_root=?`, link.root, link.agentID, link.key, link.sandboxID)
		if err != nil {
			return fmt.Errorf("agent v9 convert legacy workspace %q: %w", link.key, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("agent v9 converted an unexpected number of rows for workspace %q", link.key)
		}
	}
	if err := assertAgentForeignKeys(ctx, tx); err != nil {
		return fmt.Errorf("agent v9 FK integrity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations VALUES(?,?)",
		9, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("agent v9 commit: %w", err)
	}
	return nil
}
