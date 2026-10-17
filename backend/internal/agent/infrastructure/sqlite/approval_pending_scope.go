package sqlite

import (
	"context"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// ListPendingApprovalsForWorkspace implements the ADR-0054 fair pending
// queue: only requests of exactly one Agent/workspace, mode-eligible and
// still releasable, are selected BEFORE the SQL LIMIT, so more than 50 older
// pending entries of unrelated Agents, or ineligible requests of the same
// Agent, can never hide this workspace from the anonymous run loop. The
// eligibility predicates recheck the owner-selected policy, registered
// workspace/Profile grants and current running/not-cancelled Task generation
// against the same rows ResolveApprovalByPolicy will judge, without recording
// any decision or widening the owner-only global inbox (ListPendingApprovals).
func (s *Store) ListPendingApprovalsForWorkspace(ctx context.Context, agentID agentdomain.ID, workspaceKey string, mode agentdomain.ApprovalMode, limit int) ([]agentdomain.PendingApproval, error) {
	if _, err := agentdomain.ParseID(string(agentID)); err != nil {
		return nil, err
	}
	if mode != agentdomain.ApprovalWorkspaceRead && mode != agentdomain.ApprovalTrusted {
		return nil, agentdomain.ErrForbidden
	}
	if workspaceKey != "" {
		if err := agentdomain.ValidateWorkspaceKey(workspaceKey); err != nil {
			return nil, err
		}
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT p.*,t.workspace_key,g.name
	 FROM agent_task_approvals p
	 JOIN agent_tasks t ON t.task_id=p.task_id AND t.agent_id=p.agent_id AND t.generation=p.generation
	 JOIN agents g ON g.agent_id=p.agent_id
	 WHERE p.agent_id=? AND t.workspace_key=?
	   AND p.status='pending' AND p.expires_unix_ms>?
	   AND t.status='running' AND t.cancel_requested=0
	   AND ( (?='workspace_read' AND p.action_kind='read' AND p.can_allow=1)
	         OR (?='trusted' AND p.action_kind IN ('read','edit','execute')) )
	   AND EXISTS(
	     SELECT 1 FROM agent_approval_policies pol
	     WHERE pol.agent_id=t.agent_id AND pol.workspace_key=t.workspace_key
	       AND pol.workspace_root=t.workspace_root
	       AND pol.mode=? AND ((pol.mode='workspace_read' AND pol.expires_unix_ms=0)
	         OR (pol.mode='trusted' AND pol.expires_unix_ms>?))
	   )
	   AND (SELECT COUNT(*) FROM agent_task_approvals spent
	     WHERE spent.task_id=t.task_id AND spent.decision_source='policy')<64
	   AND (t.scope_kind!='profile' OR EXISTS(
	     SELECT 1 FROM agent_grants gr WHERE gr.agent_id=t.agent_id AND gr.profile_id=t.scope_ref))
	   AND (t.workspace_key='' OR EXISTS(
	     SELECT 1 FROM agent_workspaces w WHERE w.agent_id=t.agent_id
	     AND w.workspace_key=t.workspace_key AND w.workspace_root=t.workspace_root))
	   AND (t.scope_kind!='profile' OR t.workspace_key='' OR EXISTS(
	     SELECT 1 FROM agent_workspace_grants gw WHERE gw.agent_id=t.agent_id
	     AND gw.workspace_key=t.workspace_key AND gw.profile_id=t.scope_ref))
	 ORDER BY p.created_at ASC,p.approval_id ASC LIMIT ?`,
		agentID, workspaceKey, now.UnixMilli(), mode, mode, mode, now.UnixMilli(), limit)
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
