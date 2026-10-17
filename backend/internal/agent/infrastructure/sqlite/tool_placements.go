package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func scanToolPlacement(row interface{ Scan(...any) error }) (agentdomain.ToolPlacement, error) {
	var p agentdomain.ToolPlacement
	var encoded, when string
	err := row.Scan(&p.AgentID, &p.WorkspaceKey, &p.WorkspaceRoot, &p.SandboxID, &encoded, &when)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(encoded), &p.ToolIDs); err != nil {
		return p, err
	}
	p.UpdatedAt, err = time.Parse(time.RFC3339Nano, when)
	if err != nil {
		return p, err
	}
	return p, p.Validate()
}

const placementColumns = "agent_id,workspace_key,workspace_root,sandbox_id,tool_ids_json,updated_at"

func (s *Store) GetToolPlacement(ctx context.Context, id agentdomain.ID, key string) (agentdomain.ToolPlacement, error) {
	if _, err := agentdomain.ParseID(string(id)); err != nil {
		return agentdomain.ToolPlacement{}, err
	}
	if key != "" {
		if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
			return agentdomain.ToolPlacement{}, err
		}
	}
	p, err := scanToolPlacement(s.db.QueryRowContext(ctx, `SELECT `+placementColumns+`
  FROM agent_tool_placements WHERE agent_id=? AND workspace_key=?`, id, key))
	if errors.Is(err, sql.ErrNoRows) {
		return agentdomain.ToolPlacement{}, agentdomain.ErrNotFound
	}
	return p, err
}
func (s *Store) ListToolPlacements(ctx context.Context, id agentdomain.ID) ([]agentdomain.ToolPlacement, error) {
	if _, err := agentdomain.ParseID(string(id)); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+placementColumns+`
  FROM agent_tool_placements WHERE agent_id=? ORDER BY workspace_key`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	values := []agentdomain.ToolPlacement{}
	for rows.Next() {
		item, err := scanToolPlacement(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, item)
	}
	return values, rows.Err()
}

// PutToolPlacement fences stale workspace aliases inside the atomic SQLite
// insert/update. Runtime validates owner, Sandbox and canonical suite first;
// no provider, Sandbox or Tool can be created by this configuration change.
func (s *Store) PutToolPlacement(ctx context.Context, p agentdomain.ToolPlacement) error {
	if err := p.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(p.ToolIDs)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO agent_tool_placements(`+placementColumns+`)
 SELECT ?,?,?,?,?,? FROM agents a
 WHERE a.agent_id=? AND (
  (?='' AND a.workspace_root=?) OR
  (?<>'' AND EXISTS(SELECT 1 FROM agent_workspaces w
    WHERE w.agent_id=a.agent_id AND w.workspace_key=? AND w.workspace_root=?))
 )
 ON CONFLICT(agent_id,workspace_key) DO UPDATE SET
  workspace_root=excluded.workspace_root,
  sandbox_id=excluded.sandbox_id,
  tool_ids_json=excluded.tool_ids_json,
  updated_at=excluded.updated_at`,
		string(p.AgentID), p.WorkspaceKey, p.WorkspaceRoot, p.SandboxID, string(data),
		p.UpdatedAt.UTC().Format(time.RFC3339Nano), string(p.AgentID),
		p.WorkspaceKey, p.WorkspaceRoot, p.WorkspaceKey, p.WorkspaceKey, p.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("%w: persist governed tool placement: %w", agentdomain.ErrConflict, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return agentdomain.ErrConflict
	}
	return nil
}
func (s *Store) DeleteToolPlacement(ctx context.Context, id agentdomain.ID, key string) error {
	if _, err := agentdomain.ParseID(string(id)); err != nil {
		return err
	}
	if key != "" {
		if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
			return err
		}
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM agent_tool_placements WHERE agent_id=? AND workspace_key=?`, id, key)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return agentdomain.ErrNotFound
	}
	return nil
}
