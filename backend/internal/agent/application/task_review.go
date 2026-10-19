package application

import (
	"context"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// ReviewUnknownTask records only a reversible owner handling disposition.
// It never changes the original task's outcome, nor starts a backend.
func (s *Service) ReviewUnknownTask(ctx context.Context, id agentdomain.TaskID, handling string, caller gov.Context) (agentdomain.Task, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Task{}, err
	}
	if handling != agentdomain.UnknownHandledElsewhere && handling != agentdomain.UnknownNeedsReview {
		return agentdomain.Task{}, agentdomain.ErrInvalid
	}
	task, _, err := s.authorizedTask(ctx, id, caller)
	if err != nil {
		return agentdomain.Task{}, err
	}
	if task.Status != agentdomain.TaskUnknown {
		return agentdomain.Task{}, agentdomain.ErrConflict
	}
	if err := s.store.RecordUnknownDisposition(ctx, id, handling); err != nil {
		return agentdomain.Task{}, err
	}
	return s.Task(ctx, id, caller)
}

// resumeAvailability is a side-effect-free owner preview. It is intentionally
// narrower than actual task admission and does not authorize execution.
// The definitive Resume action independently revalidates Policy, ownership,
// workspace, native identity and persisted execution evidence.
func (s *Service) resumeAvailability(ctx context.Context, task agentdomain.Task, agent agentdomain.Agent, caller gov.Context) *agentdomain.ResumeAvailability {
	var reason string
	switch {
	case task.Status == agentdomain.TaskUnknown:
		reason = "unknown_outcome"
	case task.Status != agentdomain.TaskCompleted || task.CancelRequested:
		reason = "not_completed"
	case !task.OwnedBy(string(caller.Principal().Kind()), caller.Principal().Reference(),
		string(caller.Scope().Kind()), caller.Scope().Reference()):
		reason = "original_profile_required"
	case agent.Spec().Backend == agentdomain.BackendAntigravity:
		reason = "backend_unavailable"
	case agent.Spec().Isolation != "host":
		reason = "restricted_session"
	case agent.Spec().WorkspaceRoot != task.WorkspaceRoot && task.WorkspaceKey == "":
		reason = "workspace_changed"
	default:
		if task.WorkspaceKey != "" {
			root, err := s.store.Workspace(ctx, task.AgentID, task.WorkspaceKey)
			if err != nil || root != task.WorkspaceRoot {
				reason = "workspace_changed"
				break
			}
		}
		plan, err := s.store.ResumePlan(ctx, task.ID)
		if err == nil && plan.Valid() && plan.AgentID == task.AgentID &&
			plan.Backend == agent.Spec().Backend && plan.Isolation == "host" &&
			plan.WorkspaceRoot == task.WorkspaceRoot &&
			plan.PrincipalKind == task.PrincipalKind && plan.PrincipalRef == task.PrincipalRef &&
			plan.ScopeKind == task.ScopeKind && plan.ScopeRef == task.ScopeRef {
			return &agentdomain.ResumeAvailability{Eligible: true, Reason: "candidate"}
		}
		reason = "native_session_unavailable"
	}
	return &agentdomain.ResumeAvailability{Eligible: false, Reason: reason}
}
