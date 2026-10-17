package sqlite

import (
	"context"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// ResolveApprovalByPolicy is for an EXISTING pending human-visible request
// after the owner explicitly enabled a matching workspace policy. Its SQL
// rechecks the live policy/Task/grants atomically and records policy provenance.
// Ordinary DecideApproval remains read-only and never expands its authority.
func (s *Store) ResolveApprovalByPolicy(ctx context.Context, id agentdomain.ApprovalID, mode agentdomain.ApprovalMode) error {
	if _, err := agentdomain.ParseApprovalID(string(id)); err != nil {
		return err
	}
	if mode != agentdomain.ApprovalWorkspaceRead && mode != agentdomain.ApprovalTrusted {
		return agentdomain.ErrForbidden
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE agent_task_approvals AS a
 SET status='allowed_once',resolved_at=?,decision_source='policy'
 WHERE a.approval_id=? AND a.status='pending' AND a.expires_unix_ms>?
 AND ( (?='workspace_read' AND a.action_kind='read' AND a.can_allow=1)
       OR (?='trusted' AND a.action_kind IN ('read','edit','execute')))
 AND EXISTS(
  SELECT 1 FROM agent_tasks t
  JOIN agent_approval_policies p
    ON p.agent_id=t.agent_id AND p.workspace_key=t.workspace_key AND p.workspace_root=t.workspace_root
  WHERE t.task_id=a.task_id AND t.agent_id=a.agent_id AND t.generation=a.generation
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
 )`,
		now.Format(time.RFC3339Nano), id, now.UnixMilli(), mode, mode, mode, now.UnixMilli())
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return agentdomain.ErrConflict
	}
	return nil
}
