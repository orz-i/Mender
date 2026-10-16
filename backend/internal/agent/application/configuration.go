package application

import (
	"context"
	"fmt"
	"path/filepath"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func (s *Service) SetWorkspaceResolver(resolver WorkspaceResolver) error {
	if s == nil || resolver == nil {
		return agentdomain.ErrUnavailable
	}
	s.workspaceResolver = resolver
	return nil
}
func (s *Service) validateTarget(a agentdomain.Agent, target WorkspaceTarget) error {
	spec := a.Spec()
	if target.ID == "" || target.Backend != "local_workspace" ||
		target.OwnerKind != spec.OwnerKind || target.OwnerRef != spec.OwnerRef ||
		(target.Isolation != "host" && target.Isolation != "restricted_host") ||
		(target.Isolation == "restricted_host" && spec.Isolation == "host") ||
		!filepath.IsAbs(target.Root) || filepath.Clean(target.Root) != target.Root {
		return fmt.Errorf("%w: local workspace owner/backend/isolation mismatch", agentdomain.ErrForbidden)
	}
	return nil
}
func (s *Service) UpdateDefaultModel(ctx context.Context, id agentdomain.ID, model string, caller gov.Context) (agentdomain.Agent, error) {
	if err := s.requireOwner(caller); err != nil {
		return agentdomain.Agent{}, err
	}
	if err := agentdomain.ValidateModel(model); err != nil {
		return agentdomain.Agent{}, err
	}
	a, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return agentdomain.Agent{}, err
	}
	if !a.OwnedBy(string(s.owner.Kind()), s.owner.Reference()) {
		return agentdomain.Agent{}, agentdomain.ErrForbidden
	}
	if err := s.store.SetDefaultModel(ctx, id, model); err != nil {
		return agentdomain.Agent{}, err
	}
	return s.store.GetAgent(ctx, id)
}

// BindWorkspace is owner-only and references an existing, local Sandbox.
// No new agent, provider, or filesystem resource is created by this operation.
func (s *Service) BindWorkspace(ctx context.Context, id agentdomain.ID, key, sandboxID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if err := agentdomain.ValidateWorkspaceKey(key); err != nil {
		return err
	}
	a, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return err
	}
	if s.workspaceResolver == nil {
		return agentdomain.ErrUnavailable
	}
	target, err := s.workspaceResolver(ctx, sandboxID)
	if err != nil {
		return err
	}
	if err := s.validateTarget(a, target); err != nil {
		return err
	}
	// Never silently trust a stale/symlinked external location.
	actual, err := filepath.EvalSymlinks(target.Root)
	if err != nil {
		return fmt.Errorf("%w: sandbox root unavailable: %v", agentdomain.ErrUnavailable, err)
	}
	if actual != target.Root {
		return agentdomain.ErrUnavailable
	}
	return s.store.BindWorkspace(ctx, id, key, sandboxID)
}
func (s *Service) Workspaces(ctx context.Context, id agentdomain.ID, caller gov.Context) ([]agentdomain.WorkspaceBinding, error) {
	if err := s.requireOwner(caller); err != nil {
		return nil, err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return nil, err
	}
	return s.store.ListWorkspaces(ctx, id)
}
func (s *Service) UnbindWorkspace(ctx context.Context, id agentdomain.ID, key string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if _, err := s.store.GetAgent(ctx, id); err != nil {
		return err
	}
	return s.store.UnbindWorkspace(ctx, id, key)
}
func (s *Service) GrantWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if s.profileExists == nil {
		return agentdomain.ErrUnavailable
	}
	if _, err := s.store.Workspace(ctx, id, key); err != nil {
		return err
	}
	if err := s.profileExists(ctx, profileID); err != nil {
		return agentdomain.ErrInvalid
	}
	return s.store.GrantWorkspace(ctx, id, key, profileID)
}
func (s *Service) RevokeWorkspace(ctx context.Context, id agentdomain.ID, key, profileID string, caller gov.Context) error {
	if err := s.requireOwner(caller); err != nil {
		return err
	}
	if _, err := s.store.Workspace(ctx, id, key); err != nil {
		return err
	}
	return s.store.RevokeWorkspace(ctx, id, key, profileID)
}
