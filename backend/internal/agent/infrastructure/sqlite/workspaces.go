package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func (s *Store) SetDefaultModel(ctx context.Context, id agentdomain.ID, model string) error {
	if err := agentdomain.ValidateModel(model); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, "UPDATE agents SET default_model=? WHERE agent_id=?", model, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return agentdomain.ErrNotFound
	}
	return nil
}

// BindWorkspace never creates a workspace; it only references a preapproved local Sandbox.
// Caller must validate Sandbox owner, backend, and canonical root first.
func (s *Store) BindWorkspace(ctx context.Context, id agentdomain.ID, key, sandboxID string) error {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_workspaces(agent_id,workspace_key,sandbox_id,created_at)
 VALUES(?,?,?,?)`, id, key, sandboxID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("%w: workspace binding: %v", agentdomain.ErrConflict, err)
	}
	return nil
}
func (s *Store) Workspace(ctx context.Context, id agentdomain.ID, key string) (string, error) {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return "", err
	}
	var sandboxID string
	err := s.db.QueryRowContext(ctx, "SELECT sandbox_id FROM agent_workspaces WHERE agent_id=? AND workspace_key=?", id, key).Scan(&sandboxID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", agentdomain.ErrNotFound
	}
	return sandboxID, err
}
func (s *Store) ListWorkspaces(ctx context.Context, id agentdomain.ID) ([]agentdomain.WorkspaceBinding, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT workspace_key,sandbox_id FROM agent_workspaces WHERE agent_id=? ORDER BY workspace_key", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []agentdomain.WorkspaceBinding{}
	for rows.Next() {
		var b agentdomain.WorkspaceBinding
		if err := rows.Scan(&b.Key, &b.SandboxID); err != nil {
			return nil, err
		}
		b.ProfileIDs = []string{}
		grantRows, err := s.db.QueryContext(ctx, `SELECT profile_id FROM agent_workspace_grants WHERE agent_id=? AND workspace_key=? ORDER BY profile_id`, id, b.Key)
		if err != nil {
			return nil, err
		}
		for grantRows.Next() {
			var ref string
			if err := grantRows.Scan(&ref); err != nil {
				grantRows.Close()
				return nil, err
			}
			b.ProfileIDs = append(b.ProfileIDs, ref)
		}
		if err := grantRows.Err(); err != nil {
			grantRows.Close()
			return nil, err
		}
		grantRows.Close()
		items = append(items, b)
	}
	return items, rows.Err()
}
func (s *Store) UnbindWorkspace(ctx context.Context, id agentdomain.ID, key string) error {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	// Historical tasks preserve their effective canonical cwd; they do not
	// retain authority to run again. Active tasks keep their accepted snapshot.
	res, err := s.db.ExecContext(ctx, "DELETE FROM agent_workspaces WHERE agent_id=? AND workspace_key=?", id, key)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return agentdomain.ErrNotFound
	}
	return nil
}
func (s *Store) HasWorkspaceGrant(ctx context.Context,id agentdomain.ID,key,profileID string)(bool,error){
 if err:=agentdomain.ValidateWorkspaceKey(key);err!=nil{return false,err}
 var present bool
 err:=s.db.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM agent_workspace_grants WHERE agent_id=? AND workspace_key=? AND profile_id=?)`,id,key,profileID).Scan(&present)
 return present,err
}
func (s *Store) GrantWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string) error {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_workspace_grants(agent_id,workspace_key,profile_id,created_at) VALUES(?,?,?,?)`, id, key, profileID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("%w: workspace grant: %v", agentdomain.ErrConflict, err)
	}
	return nil
}
func (s *Store) RevokeWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string) error {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM agent_workspace_grants WHERE agent_id=? AND workspace_key=? AND profile_id=?", id, key, profileID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return agentdomain.ErrNotFound
	}
	return nil
}
