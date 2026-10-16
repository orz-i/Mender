package bootstrap

import (
	"context"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

// ResolveWorkspaceTarget only reads a previously owner-registered alias.
// Execution still requires independent Sandbox Policy, Profile grant and lease.
func (s *Sandboxes) ResolveWorkspaceTarget(ctx context.Context, providerID, key string) (sandboxdomain.ID, error) {
	if s == nil || s.store == nil {
		return "", sandboxdomain.ErrBackendUnavailable
	}
	return s.store.ResolveWorkspaceTarget(ctx, providerID, key)
}
func (r *Runtime) workspaceSourceOwner(ctx context.Context, providerID string, caller gov.Context) (sandboxdomain.ID, error) {
	if err := r.sandboxToolOwner(caller); err != nil {
		return "", err
	}
	if _, err := catalogdomain.ParseProviderID(providerID); err != nil {
		return "", err
	}
	id, err := r.Sandboxes.store.WorkspaceSuiteOwner(ctx, providerID)
	if err != nil {
		return "", err
	}
	item, err := r.Sandboxes.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if item.OwnerKind() != string(caller.Principal().Kind()) || item.OwnerRef() != caller.Principal().Reference() {
		return "", sandboxdomain.ErrNotAuthorized
	}
	return id, nil
}
func (r *Runtime) ListWorkspaceToolTargets(ctx context.Context, providerID string, caller gov.Context) ([]sandboxsqlite.WorkspaceToolTarget, error) {
	if _, err := r.workspaceSourceOwner(ctx, providerID, caller); err != nil {
		return nil, err
	}
	return r.Sandboxes.store.ListWorkspaceTargets(ctx, providerID)
}
func (r *Runtime) BindWorkspaceToolTarget(ctx context.Context, providerID, key, targetID string, caller gov.Context) error {
	defaultID, err := r.workspaceSourceOwner(ctx, providerID, caller)
	if err != nil {
		return err
	}
	id, err := sandboxdomain.ParseID(targetID)
	if err != nil {
		return err
	}
	if id == defaultID {
		return sandboxdomain.ErrConflict
	}
	target, err := r.Sandboxes.Get(ctx, id)
	if err != nil {
		return err
	}
	if target.OwnerKind() != string(caller.Principal().Kind()) || target.OwnerRef() != caller.Principal().Reference() {
		return sandboxdomain.ErrNotAuthorized
	}
	available, _, err := r.sandboxWorkspaceCapabilitySnapshot(ctx, id)
	if err != nil {
		return err
	}
	if !available {
		return sandboxdomain.ErrCapabilityUnavailable
	}
	return r.Sandboxes.store.BindWorkspaceTarget(ctx, providerID, key, id)
}
func (r *Runtime) UnbindWorkspaceToolTarget(ctx context.Context, providerID, key string, caller gov.Context) error {
	if _, err := r.workspaceSourceOwner(ctx, providerID, caller); err != nil {
		return err
	}
	return r.Sandboxes.store.UnbindWorkspaceTarget(ctx, providerID, key)
}
