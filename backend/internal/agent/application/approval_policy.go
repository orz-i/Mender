package application

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// ResolveApprovalWorkspace verifies the owner-selected existing canonical
// root. Workspaces and mode settings remain unrelated to Sandbox creation.
func (s *Service) approvalWorkspace(ctx context.Context, id agentdomain.ID, key string, owner gov.Context) (agentdomain.Agent, string, error) {
	a, err := s.Get(ctx, id, owner)
	if err != nil {
		return a, "", err
	}
	root := a.Spec().WorkspaceRoot
	if key != "" {
		if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
			return a, "", err
		}
		root, err = s.store.Workspace(ctx, id, key)
		if err != nil {
			return a, "", err
		}
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != root {
		return a, "", agentdomain.ErrUnavailable
	}
	return a, root, nil
}
func (s *Service) GetApprovalPolicy(ctx context.Context, id agentdomain.ID, key string, owner gov.Context) (agentdomain.ApprovalPolicy, error) {
	_, root, err := s.approvalWorkspace(ctx, id, key, owner)
	if err != nil {
		return agentdomain.ApprovalPolicy{}, err
	}
	policy, err := s.store.ApprovalPolicy(ctx, id, key)
	if errors.Is(err, agentdomain.ErrNotFound) {
		return agentdomain.ApprovalPolicy{AgentID: id, WorkspaceKey: key, WorkspaceRoot: root, Mode: agentdomain.ApprovalManual}, nil
	}
	if err != nil {
		return policy, err
	}
	if policy.WorkspaceRoot != root {
		return agentdomain.ApprovalPolicy{}, agentdomain.ErrConflict
	}
	if !policy.ExpiresAt.IsZero() && !time.Now().Before(policy.ExpiresAt) {
		policy.Mode = agentdomain.ApprovalManual
	}
	return policy, nil
}
func (s *Service) SetApprovalPolicy(ctx context.Context, id agentdomain.ID, key string, mode agentdomain.ApprovalMode, owner gov.Context) (agentdomain.ApprovalPolicy, error) {
	_, root, err := s.approvalWorkspace(ctx, id, key, owner)
	if err != nil {
		return agentdomain.ApprovalPolicy{}, err
	}
	if !mode.Valid() {
		return agentdomain.ApprovalPolicy{}, agentdomain.ErrInvalid
	}
	p := agentdomain.ApprovalPolicy{AgentID: id, WorkspaceKey: key, WorkspaceRoot: root, Mode: mode, UpdatedAt: time.Now().UTC()}
	if mode == agentdomain.ApprovalManual {
		if err := s.store.ClearApprovalPolicy(ctx, id, key); err != nil {
			return p, err
		}
		return p, nil
	}
	if mode == agentdomain.ApprovalTrusted {
		p.ExpiresAt = p.UpdatedAt.Add(agentdomain.MaxTrustedDuration)
	}
	if err := s.store.SetApprovalPolicy(ctx, p); err != nil {
		return p, err
	}
	return p, nil
}

// These methods are owner-only, never published through Agent delegation
// tools, HTTP Profile APIs, or model-originated tool requests.
func (s *Service) ListPendingApprovals(ctx context.Context, owner gov.Context) ([]agentdomain.PendingApproval, error) {
	if err := s.requireOwner(owner); err != nil {
		return nil, err
	}
	return s.store.ListPendingApprovals(ctx, 50)
}
func (s *Service) ListApprovalPolicies(ctx context.Context, id agentdomain.ID, owner gov.Context) ([]agentdomain.ApprovalPolicy, error) {
	if _, err := s.Get(ctx, id, owner); err != nil {
		return nil, err
	}
	policies, err := s.store.ListApprovalPolicies(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range policies {
		if !policies[i].ExpiresAt.IsZero() && !now.Before(policies[i].ExpiresAt) {
			policies[i].Mode = agentdomain.ApprovalManual
		}
	}
	return policies, nil
}

// Each ACP request is still a separate allow-once decision and a separate
// durable ledger record. No OpenCode allow_always setting is ever selected.
// The original submitting Principal, Task generation, Policy and current
// scoped owner grant are checked before recording the automatic decision.
// AllowPendingWithCurrentPolicy can release already-queued requests after an
// explicit owner mode change. The new mode does not automatically override
// unknown or unsafe requests. All actual ACP responses still select the
// originally advertised *allow_once* option for each distinct ToolCallID.
func (s *Service) AllowPendingWithCurrentPolicy(ctx context.Context, id agentdomain.ID, key string, owner gov.Context) (int, error) {
	if err := s.requireOwner(owner); err != nil {
		return 0, err
	}
	p, err := s.GetApprovalPolicy(ctx, id, key, owner)
	if err != nil {
		return 0, err
	}
	if p.Mode == agentdomain.ApprovalManual {
		return 0, nil
	}
	pending, err := s.store.ListPendingApprovals(ctx, 50)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range pending {
		a := row.Approval
		if a.AgentID != id || row.WorkspaceKey != key || !p.Allows(a.Kind, a.CanAllow, time.Now().UTC()) {
			continue
		}
		task, agent, e := s.authorizedTask(ctx, a.TaskID, owner)
		if e != nil {
			return count, e
		}
		if task.WorkspaceKey != key || task.WorkspaceRoot != p.WorkspaceRoot {
			continue
		}
		s.mu.Lock()
		waiter := s.pending[a.ID]
		active := s.active[task.ID] != nil && !s.closed
		s.mu.Unlock()
		if !active || waiter == nil {
			continue
		}
		canonical, e := filepath.EvalSymlinks(task.WorkspaceRoot)
		if e != nil || canonical != task.WorkspaceRoot {
			return count, agentdomain.ErrForbidden
		}
		if p.Mode == agentdomain.ApprovalWorkspaceRead {
			if e := revalidateReadPreview(task.WorkspaceRoot, a.Target); e != nil {
				return count, e
			}
		}
		caller, e := approvalCaller(task)
		if e != nil {
			return count, e
		}
		if e := s.allowedForApproval(ctx, agent, caller); e != nil {
			return count, e
		}
		if e := s.admit(ctx, caller, agent, gov.ActionAgentTaskSubmit); e != nil {
			return count, e
		}
		if e := s.store.ResolveApprovalByPolicy(ctx, a.ID, p.Mode); e != nil {
			if errors.Is(e, agentdomain.ErrConflict) {
				continue
			}
			return count, e
		}
		select {
		case waiter.decision <- PermissionDecision{Choice: agentdomain.ApprovalAllowOnce, AuthorizedByPolicy: true}:
		default:
		}
		count++
	}
	return count, nil
}

func (s *Service) tryAutomaticApproval(ctx context.Context, task agentdomain.Task, a agentdomain.Approval) (PermissionDecision, bool, error) {
	empty := PermissionDecision{}
	p, err := s.store.ApprovalPolicy(ctx, task.AgentID, task.WorkspaceKey)
	if errors.Is(err, agentdomain.ErrNotFound) {
		return empty, false, nil
	}
	if err != nil {
		return empty, false, err
	}
	now := time.Now().UTC()
	if p.WorkspaceRoot != task.WorkspaceRoot || !p.Allows(a.Kind, a.CanAllow, now) {
		return empty, false, nil
	}
	canonical, err := filepath.EvalSymlinks(task.WorkspaceRoot)
	if err != nil || canonical != task.WorkspaceRoot {
		return empty, false, agentdomain.ErrForbidden
	}
	if p.Mode == agentdomain.ApprovalWorkspaceRead {
		if err := revalidateReadPreview(task.WorkspaceRoot, a.Target); err != nil {
			return empty, false, err
		}
	}
	original, err := approvalCaller(task)
	if err != nil {
		return empty, false, err
	}
	agent, err := s.store.GetAgent(ctx, task.AgentID)
	if err != nil {
		return empty, false, err
	}
	if err := s.allowedForApproval(ctx, agent, original); err != nil {
		return empty, false, err
	}
	if err := s.admit(ctx, original, agent, gov.ActionAgentTaskSubmit); err != nil {
		return empty, false, err
	}
	if err := s.store.RecordAutomaticApproval(ctx, a, p.Mode); err != nil {
		return empty, false, err
	}
	return PermissionDecision{Choice: agentdomain.ApprovalAllowOnce, AuthorizedByPolicy: true}, true, nil
}
