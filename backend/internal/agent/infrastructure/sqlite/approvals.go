package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

const approvalColumns = `approval_id,task_id,agent_id,generation,session_id,tool_call_id,
 action_kind,target_path,can_allow,allow_option_id,reject_option_id,status,created_at,expires_unix_ms,resolved_at,decision_source`

func scanApproval(scan interface{ Scan(...any) error }) (agentdomain.Approval, error) {
	var a agentdomain.Approval
	var created, resolved string
	var expires int64
	var status, source string
	var canAllow int
	err := scan.Scan(&a.ID, &a.TaskID, &a.AgentID, &a.Generation, &a.SessionID, &a.ToolCallID,
		&a.Kind, &a.Target, &canAllow, &a.AllowOptionID, &a.RejectOptionID, &status, &created, &expires, &resolved, &source)
	if err != nil {
		return a, err
	}
	a.CanAllow = canAllow == 1
	a.Status = agentdomain.ApprovalStatus(status)
	a.DecisionSource = source
	if a.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return a, err
	}
	a.ExpiresAt = time.UnixMilli(expires).UTC()
	if resolved != "" {
		if a.ResolvedAt, err = time.Parse(time.RFC3339Nano, resolved); err != nil {
			return a, err
		}
	}
	return a, nil
}

// CreateApproval is an atomic, bounded task-generation admission. Inbound ACP
// metadata is not proof of any real Tool execution or authorization.
func (s *Store) CreateApproval(ctx context.Context, a agentdomain.Approval) error {
	if err := a.Validate(); err != nil {
		return err
	}
	const maxPendingPerTask = 2
	res, err := s.db.ExecContext(ctx, `INSERT INTO agent_task_approvals(`+approvalColumns+`)
 SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
 WHERE EXISTS (SELECT 1 FROM agent_tasks t
    WHERE t.task_id=? AND t.agent_id=? AND t.generation=?
     AND t.status='running' AND t.cancel_requested=0)
  AND (SELECT COUNT(*) FROM agent_task_approvals
    WHERE task_id=? AND status='pending') < ?`,
		string(a.ID), string(a.TaskID), string(a.AgentID), a.Generation, a.SessionID, a.ToolCallID,
		a.Kind, a.Target, a.CanAllow, a.AllowOptionID, a.RejectOptionID, string(a.Status),
		a.CreatedAt.UTC().Format(time.RFC3339Nano), a.ExpiresAt.UnixMilli(), "", "owner",
		string(a.TaskID), string(a.AgentID), a.Generation, string(a.TaskID), maxPendingPerTask)
	if err != nil {
		return fmt.Errorf("%w: record bounded Agent approval: %w", agentdomain.ErrConflict, err)
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
func (s *Store) GetApproval(ctx context.Context, id agentdomain.ApprovalID) (agentdomain.Approval, error) {
	if _, err := agentdomain.ParseApprovalID(string(id)); err != nil {
		return agentdomain.Approval{}, err
	}
	item, err := scanApproval(s.db.QueryRowContext(ctx, `SELECT `+approvalColumns+` FROM agent_task_approvals WHERE approval_id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return agentdomain.Approval{}, agentdomain.ErrNotFound
	}
	return item, err
}
func (s *Store) ListTaskApprovals(ctx context.Context, id agentdomain.TaskID) ([]agentdomain.Approval, error) {
	if _, err := agentdomain.ParseTaskID(string(id)); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+approvalColumns+` FROM agent_task_approvals
  WHERE task_id=? ORDER BY created_at DESC LIMIT 20`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []agentdomain.Approval{}
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

// ResolveApproval atomically checks the current Task, generation, cancellation,
// Profile AgentGrant and exact registered workspace/root/grant. An old approve
// cannot resurrect a permission after any grant was revoked or root rebound.
func (s *Store) ResolveApproval(ctx context.Context, id agentdomain.ApprovalID, status agentdomain.ApprovalStatus, policyRevision int64) error {
	if _, err := agentdomain.ParseApprovalID(string(id)); err != nil {
		return err
	}
	if status != agentdomain.ApprovalAllowedOnce && status != agentdomain.ApprovalRejected {
		return agentdomain.ErrInvalid
	}
	if status == agentdomain.ApprovalAllowedOnce && policyRevision < 1 {
		return agentdomain.ErrForbidden
	}
	now := time.Now().UTC()
	// A rejected request grants nothing. Only a positive decision checks
	// Policy v3; this keeps owner rejection possible even after Policy store
	// availability is lost, and never falls back for a positive approval.
	policyFence := ""
	args := []any{string(status), now.Format(time.RFC3339Nano), string(id), now.UnixMilli(), string(status), string(status)}
	if status == agentdomain.ApprovalAllowedOnce {
		policyFence = " AND EXISTS(SELECT 1 FROM policy_settings ps WHERE ps.singleton_id=1 AND ps.revision=?)"
		args = append(args, policyRevision)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE agent_task_approvals
  SET status=?,resolved_at=?
  WHERE approval_id=? AND status='pending' AND expires_unix_ms>?
   AND (?='rejected' OR can_allow=1)
   AND EXISTS(SELECT 1 FROM agent_tasks t
    WHERE t.task_id=agent_task_approvals.task_id
     AND t.agent_id=agent_task_approvals.agent_id
     AND t.generation=agent_task_approvals.generation
     AND t.status='running' AND t.cancel_requested=0
     AND (?='rejected' OR (
       (t.scope_kind!='profile' OR EXISTS(
         SELECT 1 FROM agent_grants g WHERE g.agent_id=t.agent_id AND g.profile_id=t.scope_ref))
       AND (t.workspace_key='' OR EXISTS(
         SELECT 1 FROM agent_workspaces w WHERE w.agent_id=t.agent_id
          AND w.workspace_key=t.workspace_key AND w.workspace_root=t.workspace_root))
       AND (t.scope_kind!='profile' OR t.workspace_key='' OR EXISTS(
         SELECT 1 FROM agent_workspace_grants gw WHERE gw.agent_id=t.agent_id
          AND gw.workspace_key=t.workspace_key AND gw.profile_id=t.scope_ref))
     ))
   )`+policyFence, args...)
	if err != nil {
		return err
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

// InvalidateApproval records expiration on a denied/abandoned live waiter.
// Terminal approvals are immutable evidence and cannot be rewritten.
func (s *Store) InvalidateApproval(ctx context.Context, id agentdomain.ApprovalID) error {
	if _, err := agentdomain.ParseApprovalID(string(id)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE agent_task_approvals SET status='expired',resolved_at=?
   WHERE approval_id=? AND status='pending'`, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

// On startup no ACP process from an older Runtime may be silently resumed.
// Invalidation is safe even when another Runtime still has an active process:
// that Runtime will fail closed instead of replaying a stored approval.
func (s *Store) InvalidateAllPendingApprovals(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_task_approvals SET status='expired',resolved_at=?
  WHERE status='pending'`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
