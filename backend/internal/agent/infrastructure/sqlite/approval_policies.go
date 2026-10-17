package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

const policyColumns = "agent_id,workspace_key,workspace_root,mode,expires_unix_ms,updated_at"

func (s *Store) ApprovalPolicy(ctx context.Context, agentID agentdomain.ID, key string) (agentdomain.ApprovalPolicy, error) {
	var p agentdomain.ApprovalPolicy
	var mode, updated string
	var expires int64
	err := s.db.QueryRowContext(ctx, "SELECT "+policyColumns+" FROM agent_approval_policies WHERE agent_id=? AND workspace_key=?", agentID, key).Scan(&p.AgentID, &p.WorkspaceKey, &p.WorkspaceRoot, &mode, &expires, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return p, agentdomain.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	p.Mode = agentdomain.ApprovalMode(mode)
	if expires != 0 {
		p.ExpiresAt = time.UnixMilli(expires).UTC()
	}
	p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return p, err
}
func (s *Store) ListApprovalPolicies(ctx context.Context, id agentdomain.ID) ([]agentdomain.ApprovalPolicy, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT workspace_key FROM agent_approval_policies WHERE agent_id=? ORDER BY workspace_key", id)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]agentdomain.ApprovalPolicy, 0, len(keys))
	for _, key := range keys {
		p, e := s.ApprovalPolicy(ctx, id, key)
		if e != nil {
			return nil, e
		}
		result = append(result, p)
	}
	return result, nil
}

// One canonical workspace root is pinned to each explicit owner grant. The
// write predicate checks the real registered key/root, never a client path.
func (s *Store) SetApprovalPolicy(ctx context.Context, p agentdomain.ApprovalPolicy) error {
	if err := p.Validate(time.Now().UTC()); err != nil {
		return err
	}
	var expires int64
	if !p.ExpiresAt.IsZero() {
		expires = p.ExpiresAt.UnixMilli()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO agent_approval_policies(
 agent_id,workspace_key,workspace_root,mode,expires_unix_ms,updated_at)
 SELECT ?,?,?,?,?,? WHERE EXISTS(
 SELECT 1 FROM agents a WHERE a.agent_id=? AND
 ((?='' AND a.workspace_root=?) OR
 EXISTS(SELECT 1 FROM agent_workspaces w WHERE w.agent_id=a.agent_id AND w.workspace_key=? AND w.workspace_root=?))
 )
 ON CONFLICT(agent_id,workspace_key) DO UPDATE SET
 workspace_root=excluded.workspace_root,mode=excluded.mode,
 expires_unix_ms=excluded.expires_unix_ms,updated_at=excluded.updated_at`,
		p.AgentID, p.WorkspaceKey, p.WorkspaceRoot, p.Mode, expires, p.UpdatedAt.UTC().Format(time.RFC3339Nano),
		p.AgentID, p.WorkspaceKey, p.WorkspaceRoot, p.WorkspaceKey, p.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("set Agent approval policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return agentdomain.ErrConflict
	}
	return nil
}
func (s *Store) ClearApprovalPolicy(ctx context.Context, agentID agentdomain.ID, key string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM agent_approval_policies WHERE agent_id=? AND workspace_key=?", agentID, key)
	return err
}

// RecordAutomaticApproval writes an immutable, automatic allow-once receipt.
// The owner-selected policy and EXACT Task caller/grants are rechecked inside
// the same SQLite write; a concurrently revoked grant cannot be reused.
func (s *Store) RecordAutomaticApproval(ctx context.Context, a agentdomain.Approval, mode agentdomain.ApprovalMode) error {
	if err := a.Validate(); err != nil {
		return err
	}
	allowed := false
	switch mode {
	case agentdomain.ApprovalWorkspaceRead:
		allowed = a.Kind == "read" && a.CanAllow
	case agentdomain.ApprovalTrusted:
		allowed = a.Kind == "read" || a.Kind == "edit" || a.Kind == "execute"
	}
	if !allowed {
		return agentdomain.ErrForbidden
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*)
 FROM agent_tasks t
 JOIN agent_approval_policies p ON p.agent_id=t.agent_id AND p.workspace_key=t.workspace_key
   AND p.workspace_root=t.workspace_root
 WHERE t.task_id=? AND t.agent_id=? AND t.generation=?
   AND t.status='running' AND t.cancel_requested=0
   AND p.mode=? AND ((p.mode='workspace_read' AND p.expires_unix_ms=0)
    OR (p.mode='trusted' AND p.expires_unix_ms>?))
   AND (t.scope_kind!='profile' OR EXISTS(
     SELECT 1 FROM agent_grants g WHERE g.agent_id=t.agent_id AND g.profile_id=t.scope_ref))
   AND (t.workspace_key='' OR EXISTS(
     SELECT 1 FROM agent_workspaces w WHERE w.agent_id=t.agent_id
     AND w.workspace_key=t.workspace_key AND w.workspace_root=t.workspace_root))
   AND (t.scope_kind!='profile' OR t.workspace_key='' OR EXISTS(
     SELECT 1 FROM agent_workspace_grants gw WHERE gw.agent_id=t.agent_id
     AND gw.workspace_key=t.workspace_key AND gw.profile_id=t.scope_ref))
   AND (SELECT COUNT(*) FROM agent_task_approvals q WHERE q.task_id=t.task_id AND q.decision_source='policy')<64`,
		a.TaskID, a.AgentID, a.Generation, mode, now.UnixMilli()).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return agentdomain.ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_task_approvals(
  approval_id,task_id,agent_id,generation,session_id,tool_call_id,
  action_kind,target_path,can_allow,allow_option_id,reject_option_id,
  status,created_at,expires_unix_ms,resolved_at,decision_source)
  VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.TaskID, a.AgentID, a.Generation, a.SessionID, a.ToolCallID,
		a.Kind, a.Target, a.CanAllow, a.AllowOptionID, a.RejectOptionID,
		string(agentdomain.ApprovalAllowedOnce), a.CreatedAt.Format(time.RFC3339Nano),
		a.ExpiresAt.UnixMilli(), now.Format(time.RFC3339Nano), "policy")
	if err != nil {
		return fmt.Errorf("%w: automatic Agent approval: %w", agentdomain.ErrConflict, err)
	}
	return tx.Commit()
}

func (s *Store) ListPendingApprovals(ctx context.Context, limit int) ([]agentdomain.PendingApproval, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT p.*,t.workspace_key,g.name
 FROM agent_task_approvals p
 JOIN agent_tasks t ON t.task_id=p.task_id AND t.agent_id=p.agent_id AND t.generation=p.generation
 JOIN agents g ON g.agent_id=p.agent_id
 WHERE p.status='pending' AND p.expires_unix_ms>?
   AND t.status='running' AND t.cancel_requested=0
 ORDER BY p.created_at ASC,p.approval_id ASC LIMIT ?`, time.Now().UTC().UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []agentdomain.PendingApproval{}
	for rows.Next() {
		var row agentdomain.PendingApproval
		var a agentdomain.Approval
		var created, resolved, status, source string
		var expires int64
		var canAllow int
		if err := rows.Scan(&a.ID, &a.TaskID, &a.AgentID, &a.Generation, &a.SessionID, &a.ToolCallID,
			&a.Kind, &a.Target, &canAllow, &a.AllowOptionID, &a.RejectOptionID,
			&status, &created, &expires, &resolved, &source, &row.WorkspaceKey, &row.AgentName); err != nil {
			return nil, err
		}
		a.CanAllow = canAllow == 1
		a.Status = agentdomain.ApprovalStatus(status)
		a.DecisionSource = source
		a.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		a.ExpiresAt = time.UnixMilli(expires).UTC()
		row.Approval = a
		result = append(result, row)
	}
	return result, rows.Err()
}
