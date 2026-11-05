package application

import (
	"context"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type Admission func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error
type ProfileExists func(context.Context, string) error

// agentAuthority keeps owner and delegated Profile identity rules identical
// across resource, Task and native approval use cases.
type agentAuthority struct {
	owner  gov.Principal
	grants ProfileGrantLookup
}

// taskAccess rechecks current grants and immutable submitting identity. It
// shares no execution state and does not authorize a new native process.
type taskAccess struct {
	*agentAuthority
	store TaskLookup
}

func (s *agentAuthority) isOwner(caller gov.Context) bool {
	return caller.Validate() == nil && caller.Principal() == s.owner &&
		caller.Scope().Kind() == gov.ScopeCatalog &&
		(caller.Origin() == gov.OriginCLI || caller.Origin() == gov.OriginDesktop ||
			caller.Origin() == gov.OriginManagementHTTP || caller.Origin() == gov.OriginInternal)
}
func (s *agentAuthority) requireOwner(caller gov.Context) error {
	if !s.isOwner(caller) {
		return agentdomain.ErrForbidden
	}
	return nil
}
func (s *agentAuthority) isTrustedProfile(caller gov.Context) string {
	if caller.Validate() != nil || caller.Origin() != gov.OriginMCP ||
		caller.Principal().Kind() != gov.PrincipalService || caller.Scope().Kind() != gov.ScopeProfile {
		return ""
	}
	id := caller.Scope().Reference()
	if _, err := profiledomain.ParseProfileID(id); err != nil || caller.Principal().Reference() != "profile:"+id {
		return ""
	}
	return id
}
func (s *agentAuthority) allowed(ctx context.Context, agent agentdomain.Agent, caller gov.Context) (string, error) {
	if s.isOwner(caller) && agent.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return "", nil
	}
	profileID := s.isTrustedProfile(caller)
	if profileID == "" {
		return "", agentdomain.ErrForbidden
	}
	grant, err := s.grants.HasProfileGrant(ctx, agent.ID(), profileID)
	if err != nil {
		return "", err
	}
	if !grant {
		return "", agentdomain.ErrForbidden
	}
	return profileID, nil
}

func (s *taskAccess) authorizedTask(ctx context.Context, id agentdomain.TaskID, caller gov.Context) (agentdomain.Task, agentdomain.Agent, error) {
	task, err := s.store.GetTask(ctx, id)
	if err != nil {
		return task, agentdomain.Agent{}, err
	}
	agent, err := s.store.GetAgent(ctx, task.AgentID)
	if err != nil {
		return task, agent, err
	}
	if s.isOwner(caller) && agent.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return task, agent, nil
	}
	profileID, err := s.allowed(ctx, agent, caller)
	if err != nil {
		return task, agent, err
	}
	if profileID == "" || !task.OwnedBy(string(caller.Principal().Kind()), caller.Principal().Reference(),
		string(caller.Scope().Kind()), caller.Scope().Reference()) {
		return task, agent, agentdomain.ErrForbidden
	}
	return task, agent, nil
}

func (s *taskAccess) agentForOwner(ctx context.Context, id agentdomain.ID, caller gov.Context) (agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Agent{}, err
	}
	return s.store.GetAgent(ctx, id)
}
