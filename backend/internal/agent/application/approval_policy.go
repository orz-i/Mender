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
	// Each page is filtered by exact AgentID/workspace and action eligibility
	// BEFORE SQL LIMIT. Bounded batches prevent one owner click from starving
	// other owners or keeping the UI blocked on an unlimited queue.
	const batchSize, maxBatches = 50, 4
	count := 0
	var after agentdomain.ApprovalID
	for page := 0; page < maxBatches; page++ {
		pending, err := s.store.ListPendingApprovalsForWorkspace(ctx, id, key, p.Mode, after, batchSize)
		if err != nil {
			return count, err
		}
		if len(pending) == 0 {
			break
		}
		// The cursor moves by the immutable pending row identity, not the
		// number of approvals resolved. Invalid or unavailable previews stay
		// in the owner inbox without blocking later valid requests.
		after = pending[len(pending)-1].Approval.ID
		for _, row := range pending {
			a := row.Approval
			if !p.Allows(a.Kind, a.CanAllow, time.Now().UTC()) {
				continue
			}
			task, agent, e := s.authorizedTask(ctx, a.TaskID, owner)
			if errors.Is(e, agentdomain.ErrConflict) {
				continue
			}
			if e != nil {
				return count, e
			}
			if task.WorkspaceKey != key || task.WorkspaceRoot != p.WorkspaceRoot {
				continue
			}
			canonical, e := filepath.EvalSymlinks(task.WorkspaceRoot)
			if e != nil || canonical != task.WorkspaceRoot {
				return count, agentdomain.ErrForbidden
			}
			if p.Mode == agentdomain.ApprovalWorkspaceRead {
				if e := revalidateReadPreview(task.WorkspaceRoot, a.Target); e != nil {
					// A missing or invalid preview is not an authorization.
					// Leave it pending for the owner; proceed to other files.
					continue
				}
			}
			caller, e := approvalCaller(task)
			if e != nil {
				return count, e
			}
			policyRevision, e := s.store.CurrentPolicyRevision(ctx)
			if e != nil {
				return count, e
			}
			if e := s.allowedForApproval(ctx, agent, caller); e != nil {
				return count, e
			}
			if e := s.admit(ctx, caller, agent, gov.ActionAgentTaskSubmit); e != nil {
				return count, e
			}
			e = s.resolveWaitingApproval(ctx, a.ID, task.ID,
				PermissionDecision{Choice: agentdomain.ApprovalAllowOnce, AuthorizedByPolicy: true},
				func(commitCtx context.Context) error {
					return s.store.ResolveApprovalByPolicy(commitCtx, a.ID, p.Mode, policyRevision)
				})
			if errors.Is(e, agentdomain.ErrConflict) {
				continue
			}
			if e != nil {
				return count, e
			}
			count++
		}
		if len(pending) < batchSize {
			break
		}
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
	policyRevision, err := s.store.CurrentPolicyRevision(ctx)
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
	if err := s.store.RecordAutomaticApproval(ctx, a, p.Mode, policyRevision); err != nil {
		if errors.Is(err, agentdomain.ErrConflict) {
			// A changed Policy, grant, expiry or per-task budget must never
			// reuse the old allow. Still-live requests enter manual review.
			return empty, false, nil
		}
		return empty, false, err
	}
	return PermissionDecision{Choice: agentdomain.ApprovalAllowOnce, AuthorizedByPolicy: true}, true, nil
}
