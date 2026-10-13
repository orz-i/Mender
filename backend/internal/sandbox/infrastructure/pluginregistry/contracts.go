package pluginregistry

import (
	"slices"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// VerifiedWorkspaceContracts derives a read-only, exact-pinned, typed Workspace
// v1 offer set. A built-in must have its host-verified immutable contracts;
// a native Sandbox Protocol v1 plugin can offer only its verified four
// Workspace operations, mapped by the host to v1/inline. No plugin RPC,
// credential resolution, remote connection or backend.Open occurs here.
// Eligibility still requires observed Sandbox readiness and, for execution,
// the usual Router admission and durable lease.
func (r *Registry) VerifiedWorkspaceContracts(kind sandboxdomain.BackendKind,
	binding sandboxdomain.BackendBinding) ([]platformplugin.CapabilityContract, error) {
	if r == nil || !r.Available(kind, binding) {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	r.mu.RLock()
	var entry registered
	if kind == sandboxdomain.BackendExternal {
		entry = r.external[binding.InstallationID]
	} else {
		entry = r.byKind[kind]
	}
	r.mu.RUnlock()
	if entry.backend == nil || entry.backend.Kind() != kind {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	result := make([]platformplugin.CapabilityContract, 0, 4)
	for _, op := range []sandboxdomain.Capability{
		sandboxdomain.CapabilityRead, sandboxdomain.CapabilitySearch,
		sandboxdomain.CapabilityEdit, sandboxdomain.CapabilityShell,
	} {
		// ResolveBackend is side-effect-free and cross-checks the pinned
		// descriptor against the live typed backend's actual capability set.
		if _, err := r.ResolveBackend(kind, binding, op); err != nil {
			continue
		}
		contract := platformplugin.CapabilityContract{ID: platformplugin.CapabilityID(op),
			Version: platformplugin.CapabilityVersion{Major: 1}, Mode: platformplugin.ExecutionInline}
		if kind != sandboxdomain.BackendExternal && !slices.ContainsFunc(entry.descriptor.CapabilityContracts,
			func(declared platformplugin.CapabilityContract) bool {
				return declared.ID == contract.ID && declared.Version == contract.Version &&
					declared.Mode == contract.Mode && len(declared.Guarantees) == 0
			}) {
			continue
		}
		result = append(result, contract)
	}
	return result, nil
}
