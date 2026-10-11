// Package pluginregistry binds trusted Sandbox Backend extensions to real
// execution implementations. It never grants acquisition or lifecycle authority.
package pluginregistry

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const builtinPackageVersion = "1.0.0"
const mainExtension platformplugin.ExtensionID = "main"

// Entry describes one registered executable Backend implementation. Its
// Descriptor is immutable management metadata, not a security assertion.
type Entry struct {
	Kind       sandboxdomain.BackendKind
	Descriptor platformplugin.Descriptor
}

// Registry is the only trusted runtime mapping from a Sandbox Backend kind to
// its extension implementation. In C1 it binds the existing persisted kinds;
// C2 will persist exact extension identities instead of resolving by kind.
type Registry struct {
	mu          sync.RWMutex
	byKind      map[sandboxdomain.BackendKind]registered
	byExtension map[platformplugin.ID]sandboxdomain.BackendKind
}

type registered struct {
	descriptor platformplugin.Descriptor
	backend    sandboxapp.Backend
}

func NewRegistry() *Registry {
	return &Registry{
		byKind:      make(map[sandboxdomain.BackendKind]registered),
		byExtension: make(map[platformplugin.ID]sandboxdomain.BackendKind),
	}
}

// BuiltinDescriptor derives advertised tool-operation capabilities from the
// implementation supplied by the host. It cannot claim unseen capabilities.
func BuiltinDescriptor(backend sandboxapp.Backend) (platformplugin.Descriptor, error) {
	if backend == nil {
		return platformplugin.Descriptor{}, fmt.Errorf("sandbox plugin: backend is required")
	}
	var id platformplugin.ID
	var display, description string
	switch backend.Kind() {
	case sandboxdomain.BackendLocalWorkspace:
		id = "io.mender.sandbox.local-workspace"
		display = "Local Workspace"
		description = "Trusted host workspace backend; host or restricted-host, not container isolation"
	case sandboxdomain.BackendDocker:
		id = "io.mender.sandbox.docker"
		display = "Docker Engine"
		description = "Trusted opt-in Docker Engine backend with container isolation checks"
	case sandboxdomain.BackendDaytona:
		id = "io.mender.sandbox.daytona"
		display = "Daytona (experimental)"
		description = "Opt-in remote Sandbox backend; live integration is not accepted"
	default:
		return platformplugin.Descriptor{}, fmt.Errorf("sandbox plugin: no built-in identity for backend %q", backend.Kind())
	}
	d := platformplugin.Descriptor{
		PluginID: id, PackageVersion: builtinPackageVersion,
		Extension:   platformplugin.Extension{ID: mainExtension, Kind: platformplugin.SandboxBackend, ProtocolVersion: 1},
		DisplayName: display, Description: description, ConfigVersion: 1,
		Capabilities: capabilityNames(backend.Capabilities()),
	}
	return d, d.Validate()
}

func capabilityNames(cap sandboxdomain.Capabilities) []string {
	result := make([]string, 0, 4)
	if cap.Read {
		result = append(result, string(sandboxdomain.CapabilityRead))
	}
	if cap.Search {
		result = append(result, string(sandboxdomain.CapabilitySearch))
	}
	if cap.Edit {
		result = append(result, string(sandboxdomain.CapabilityEdit))
	}
	if cap.Shell {
		result = append(result, string(sandboxdomain.CapabilityShell))
	}
	return result
}

// Register rejects false capability claims and duplicate driver assignments.
// C1 admits built-in drivers only; the external process trust boundary and
// persisted InstallationID binding are deliberately deferred to C2/C3.
func (r *Registry) Register(backend sandboxapp.Backend, descriptor platformplugin.Descriptor) error {
	if r == nil {
		return fmt.Errorf("sandbox plugin: registry unavailable")
	}
	if backend == nil {
		return fmt.Errorf("sandbox plugin: backend required")
	}
	descriptor = descriptor.Clone()
	if err := descriptor.Validate(); err != nil {
		return fmt.Errorf("sandbox plugin: descriptor: %w", err)
	}
	if descriptor.Extension.Kind != platformplugin.SandboxBackend || descriptor.Extension.ID != mainExtension {
		return fmt.Errorf("sandbox plugin: expected sandbox.backend/main extension")
	}
	kind := backend.Kind()
	if err := kind.Validate(); err != nil {
		return err
	}
	expected, err := BuiltinDescriptor(backend)
	if err != nil {
		return err
	}
	if descriptor.PluginID != expected.PluginID || descriptor.Extension != expected.Extension ||
		descriptor.ConfigVersion != expected.ConfigVersion || descriptor.PackageVersion != expected.PackageVersion {
		return fmt.Errorf("sandbox plugin: wrong built-in extension binding for %s", kind)
	}
	actual := capabilityNames(backend.Capabilities())
	claimed := slices.Clone(descriptor.Capabilities)
	slices.Sort(actual)
	slices.Sort(claimed)
	if !slices.Equal(actual, claimed) {
		return fmt.Errorf("sandbox plugin: descriptor capabilities do not match backend implementation")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byKind[kind]; ok {
		return fmt.Errorf("sandbox plugin: backend kind %s already registered", kind)
	}
	if _, ok := r.byExtension[descriptor.PluginID]; ok {
		return fmt.Errorf("sandbox plugin: extension id %s already registered", descriptor.PluginID)
	}
	r.byKind[kind] = registered{descriptor: descriptor, backend: backend}
	r.byExtension[descriptor.PluginID] = kind
	return nil
}

// ResolveBackend is local, side-effect free, and does not acquire a lease.
// It is always called AFTER the Sandbox Router has admitted Principal/Policy.
func (r *Registry) ResolveBackend(kind sandboxdomain.BackendKind, binding sandboxdomain.BackendBinding, required sandboxdomain.Capability) (sandboxapp.Backend, error) {
	if r == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := required.Validate(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	entry, ok := r.byKind[kind]
	r.mu.RUnlock()
	if !ok || entry.backend == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if binding.Validate() != nil || binding.PluginID != entry.descriptor.PluginID ||
		binding.ExtensionID != entry.descriptor.Extension.ID || binding.InstallationID != "" {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	// Capability authority cannot grow beyond the immutable registered descriptor,
	// even if a mutable backend later begins reporting additional operations.
	if !slices.Contains(entry.descriptor.Capabilities, string(required)) ||
		!entry.backend.Capabilities().Supports(required) {
		return nil, sandboxdomain.ErrCapabilityUnavailable
	}
	if entry.backend.Kind() != kind {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return entry.backend, nil
}

func (r *Registry) Available(kind sandboxdomain.BackendKind, binding sandboxdomain.BackendBinding) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	item, ok := r.byKind[kind]
	r.mu.RUnlock()
	return ok && binding.Validate() == nil && binding.PluginID == item.descriptor.PluginID &&
		binding.ExtensionID == item.descriptor.Extension.ID && binding.InstallationID == ""
}

// Entries exposes only inert metadata, without launching drivers or probing
// remotes. Callers cannot mutate descriptor snapshots held by this Registry.
func (r *Registry) Entries() []Entry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	items := make([]Entry, 0, len(r.byKind))
	for kind, entry := range r.byKind {
		items = append(items, Entry{Kind: kind, Descriptor: entry.descriptor.Clone()})
	}
	r.mu.RUnlock()
	slices.SortFunc(items, func(a, b Entry) int { return strings.Compare(string(a.Kind), string(b.Kind)) })
	return items
}

var _ sandboxapp.BackendResolver = (*Registry)(nil)
