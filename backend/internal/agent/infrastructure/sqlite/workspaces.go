package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func (s *Store) SetDefaults(ctx context.Context, id agentdomain.ID, model, effort string) error {
	if err := agentdomain.ValidateModel(model); err != nil {
		return err
	}
	if err := agentdomain.ValidateReasoningEffort(effort); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, "UPDATE agents SET default_model=?,default_reasoning_effort=? WHERE agent_id=?", model, effort, id)
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

// BindWorkspace records an owner-approved canonical local directory on one
// Agent. The path is never an MCP Tool argument, and grants remain separate.
func (s *Store) BindWorkspace(ctx context.Context, id agentdomain.ID, key, root string) error {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	// SQLite serializes the write. A losing concurrent registration must
	// never overwrite a workspace or its Profile grants.
	result, err := s.db.ExecContext(ctx, `INSERT INTO agent_workspaces(agent_id,workspace_key,workspace_root,created_at)
 VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, id, key, root, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("register Agent workspace: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 1 {
		return nil
	}
	if count != 0 {
		return agentdomain.ErrConflict
	}
	// Resolve the precise conflict only after the rejected atomic INSERT.
	// Alias takes precedence if both the key and directory are duplicates.
	var existingKey string
	err = s.db.QueryRowContext(ctx, `SELECT workspace_key FROM agent_workspaces
	 WHERE agent_id=? AND (workspace_key=? OR workspace_root=?)
	 ORDER BY CASE WHEN workspace_key=? THEN 0 ELSE 1 END LIMIT 1`, id, key, root, key).Scan(&existingKey)
	if errors.Is(err, sql.ErrNoRows) {
		return agentdomain.ErrConflict
	}
	if err != nil {
		return err
	}
	if existingKey == key {
		return fmt.Errorf("%w: %w", agentdomain.ErrConflict, agentdomain.ErrWorkspaceAliasExists)
	}
	return fmt.Errorf("%w: %w", agentdomain.ErrConflict, agentdomain.ErrWorkspaceDirectoryExists)
}
func (s *Store) Workspace(ctx context.Context, id agentdomain.ID, key string) (string, error) {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return "", err
	}
	var root string
	err := s.db.QueryRowContext(ctx, "SELECT workspace_root FROM agent_workspaces WHERE agent_id=? AND workspace_key=?", id, key).Scan(&root)
	if errors.Is(err, sql.ErrNoRows) {
		return "", agentdomain.ErrNotFound
	}
	return root, err
}
func (s *Store) ListWorkspaces(ctx context.Context, id agentdomain.ID) ([]agentdomain.WorkspaceBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT w.workspace_key,w.workspace_root,g.profile_id FROM agent_workspaces w
 LEFT JOIN agent_workspace_grants g ON w.agent_id=g.agent_id AND w.workspace_key=g.workspace_key
 WHERE w.agent_id=? ORDER BY w.workspace_key,g.profile_id`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []agentdomain.WorkspaceBinding{}
	for rows.Next() {
		var key, root string
		var profileID sql.NullString
		if err := rows.Scan(&key, &root, &profileID); err != nil {
			return nil, err
		}
		if len(items) == 0 || items[len(items)-1].Key != key {
			items = append(items, agentdomain.WorkspaceBinding{Key: key, WorkspaceRoot: root, ProfileIDs: []string{}})
		}
		if profileID.Valid {
			items[len(items)-1].ProfileIDs = append(items[len(items)-1].ProfileIDs, profileID.String)
		}
	}
	return items, rows.Err()
}
func (s *Store) UnbindWorkspace(ctx context.Context, id agentdomain.ID, key string) error {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	// Historical tasks preserve their effective canonical cwd; they do not
	// retain authority to run again. Active tasks keep their accepted snapshot.
	res, err := s.db.ExecContext(ctx, `DELETE FROM agent_workspaces
  WHERE agent_id=? AND workspace_key=?
   AND NOT EXISTS(SELECT 1 FROM agent_tool_placements p
      WHERE p.agent_id=agent_workspaces.agent_id AND p.workspace_key=agent_workspaces.workspace_key)`, id, key)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	if n != 0 {
		return agentdomain.ErrConflict
	}
	var linked bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_tool_placements
    WHERE agent_id=? AND workspace_key=?)`, id, key).Scan(&linked); err != nil {
		return err
	}
	if linked {
		return fmt.Errorf("%w: %w", agentdomain.ErrConflict, agentdomain.ErrWorkspaceGovernedToolsEnabled)
	}
	return agentdomain.ErrNotFound
}
func (s *Store) HasWorkspaceGrant(ctx context.Context, id agentdomain.ID, key, profileID string) (bool, error) {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return false, err
	}
	var present bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_workspace_grants WHERE agent_id=? AND workspace_key=? AND profile_id=?)`, id, key, profileID).Scan(&present)
	return present, err
}
func (s *Store) GrantWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string) error {
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_workspace_grants(agent_id,workspace_key,profile_id,created_at) VALUES(?,?,?,?)`, id, key, profileID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("%w: workspace grant: %w", agentdomain.ErrConflict, err)
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
