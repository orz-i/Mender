package bootstrap

import (
	"context"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// VerifiedSSHJobContracts is an inert, host-reviewed claim about the ACTUAL
// built-in SSH Durable Jobs implementation, not about arbitrary extension
// Descriptor.Capabilities strings. It resolves no credentials or SSH network.
// A Browser, Docker, Workspace Shell or third-party Sandbox v1 extension can
// never be substituted for this exact pinned first-party implementation.
func (s *Sandboxes) VerifiedSSHJobContracts(ctx context.Context, id sandboxdomain.ID) ([]platformplugin.CapabilityContract, error) {
	if s == nil || s.store == nil || s.sshRemote == nil || s.backendPlugins == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	resource, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if resource.Backend() != sandboxdomain.BackendSSHRemote ||
		resource.Spec().Binding != sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote) ||
		!s.backendPlugins.Available(resource.Backend(), resource.Spec().Binding) {
		return nil, sandboxdomain.ErrCapabilityUnavailable
	}
	template, err := s.store.GetSSHRemoteTemplate(ctx, id)
	if err != nil || template.User != "root" {
		return nil, sandboxdomain.ErrCapabilityUnavailable
	}
	version := platformplugin.CapabilityVersion{Major: 1}
	return []platformplugin.CapabilityContract{
		{ID: platformplugin.CapabilityID(sandboxdomain.CapabilityJobSubmit), Version: version, Mode: platformplugin.ExecutionDetached,
			Guarantees: []platformplugin.ExecutionGuarantee{
				platformplugin.GuaranteeIdempotentSubmit, platformplugin.GuaranteeReconnectObservable, platformplugin.GuaranteePersistedResult}},
		{ID: platformplugin.CapabilityID(sandboxdomain.CapabilityJobObserve), Version: version, Mode: platformplugin.ExecutionInline},
		{ID: platformplugin.CapabilityID(sandboxdomain.CapabilityJobCancel), Version: version, Mode: platformplugin.ExecutionInline},
	}, nil
}
