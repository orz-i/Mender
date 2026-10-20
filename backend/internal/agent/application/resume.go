package application

import (
	"context"
	"errors"
	"strings"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// Resume creates a new explicit Task from an existing, proven-complete
// native Session. It never retries the old prompt, mutates an unknown Task,
// trusts a raw native ID from a model, or bypasses ordinary submit grants.
func (s *Service) Resume(ctx context.Context, id agentdomain.TaskID, prompt string,
	agentID agentdomain.ID, caller gov.Context) (agentdomain.Task, error) {
	if caller.Validate() != nil {
		return agentdomain.Task{}, agentdomain.ErrForbidden
	}
	if strings.TrimSpace(prompt) == "" || len(prompt) > agentdomain.MaxPromptBytes {
		return agentdomain.Task{}, agentdomain.ErrInvalid
	}
	parent, agent, err := s.authorizedTask(ctx, id, caller)
	if err != nil {
		return agentdomain.Task{}, err
	}
	if parent.AgentID != agentID {
		return agentdomain.Task{}, agentdomain.ErrForbidden
	}
	// Owner can inspect service submissions, but cannot impersonate the
	// original service Principal or take over its native conversation.
	if !parent.OwnedBy(string(caller.Principal().Kind()), caller.Principal().Reference(),
		string(caller.Scope().Kind()), caller.Scope().Reference()) {
		return agentdomain.Task{}, agentdomain.ErrForbidden
	}
	profileID, err := s.allowed(ctx, agent, caller)
	if err != nil {
		return agentdomain.Task{}, err
	}
	// Runtime-isolated HOME contents are intentionally ephemeral. Do not
	// offer cross-process continuation until durable storage has been proven
	// for the exact restricted placement without broadening Seatbelt.
	if agent.Spec().Isolation != "host" || parent.Status != agentdomain.TaskCompleted ||
		parent.CancelRequested {
		return agentdomain.Task{}, agentdomain.ErrResumeUnavailable
	}
	if parent.WorkspaceKey != "" {
		if err := agentdomain.ValidateWorkspaceKey(parent.WorkspaceKey); err != nil {
			return agentdomain.Task{}, err
		}
		root, err := s.store.Workspace(ctx, agentID, parent.WorkspaceKey)
		if err != nil {
			return agentdomain.Task{}, agentdomain.ErrResumeUnavailable
		}
		if root != parent.WorkspaceRoot {
			return agentdomain.Task{}, agentdomain.ErrResumeUnavailable
		}
		if err := validateRegisteredWorkspaceRoot(root); err != nil {
			return agentdomain.Task{}, agentdomain.ErrResumeUnavailable
		}
		if profileID != "" {
			granted, err := s.store.HasWorkspaceGrant(ctx, agentID, parent.WorkspaceKey, profileID)
			if err != nil {
				return agentdomain.Task{}, err
			}
			if !granted {
				return agentdomain.Task{}, agentdomain.ErrForbidden
			}
		}
	} else {
		if agent.Spec().WorkspaceRoot != parent.WorkspaceRoot {
			return agentdomain.Task{}, agentdomain.ErrResumeUnavailable
		}
		if err := validateRegisteredWorkspaceRoot(parent.WorkspaceRoot); err != nil {
			return agentdomain.Task{}, agentdomain.ErrResumeUnavailable
		}
	}
	for _, action := range []gov.Action{gov.ActionAgentActivate, gov.ActionAgentTaskSubmit, gov.ActionAgentTaskResume} {
		if err := s.admit(ctx, caller, agent, action); err != nil {
			return agentdomain.Task{}, err
		}
	}
	plan, err := s.store.ResumePlan(ctx, id)
	if err != nil {
		return agentdomain.Task{}, err
	}
	if !plan.Valid() || plan.AgentID != agentID || plan.Backend != agent.Spec().Backend ||
		plan.Isolation != "host" || plan.WorkspaceRoot != parent.WorkspaceRoot ||
		plan.PrincipalKind != parent.PrincipalKind || plan.PrincipalRef != parent.PrincipalRef ||
		plan.ScopeKind != parent.ScopeKind || plan.ScopeRef != parent.ScopeRef {
		return agentdomain.Task{}, agentdomain.ErrResumeUnavailable
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		return agentdomain.Task{}, err
	}
	now := time.Now().UTC()
	task := agentdomain.Task{
		ID: taskID, AgentID: agentID, Status: agentdomain.TaskQueued,
		Model: parent.Model, ReasoningEffort: parent.ReasoningEffort, TimeoutSeconds: parent.TimeoutSeconds,
		WorkspaceKey: parent.WorkspaceKey, WorkspaceRoot: parent.WorkspaceRoot,
		Origin: string(caller.Origin()), PrincipalKind: string(caller.Principal().Kind()),
		PrincipalRef: caller.Principal().Reference(),
		ScopeKind:    string(caller.Scope().Kind()), ScopeRef: caller.Scope().Reference(),
		Generation: 1, CreatedAt: now, UpdatedAt: now, ContinuedFromTaskID: id,
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return agentdomain.Task{}, agentdomain.ErrUnavailable
	}
	if err := s.store.CreateResumedTask(ctx, task, plan, maxActivePerAgent, profileID); err != nil {
		s.mu.Unlock()
		if errors.Is(err, agentdomain.ErrResumeUnavailable) {
			return agentdomain.Task{}, agentdomain.ErrResumeUnavailable
		}
		return agentdomain.Task{}, err
	}
	runCtx, cancel := context.WithTimeout(s.rootContext, task.ExecutionTimeout())
	s.active[taskID] = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go s.run(runCtx, cancel, agent, task, prompt, caller, &plan)
	return task, nil
}
