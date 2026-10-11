package plugin

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Registry stores immutable built-in typed extension descriptors. It never
// starts plugins or performs remote probes on catalogue reads.
type Registry struct {
	mu          sync.RWMutex
	descriptors map[ID]map[ExtensionID]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{descriptors: make(map[ID]map[ExtensionID]Descriptor)}
}

func (r *Registry) Register(descriptor Descriptor) error {
	if r == nil {
		return fmt.Errorf("plugin registry: nil registry")
	}
	descriptor = descriptor.Clone()
	if err := descriptor.Validate(); err != nil {
		return fmt.Errorf("plugin registry: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	extensions, ok := r.descriptors[descriptor.PluginID]
	if !ok {
		extensions = make(map[ExtensionID]Descriptor)
		r.descriptors[descriptor.PluginID] = extensions
	}
	if _, exists := extensions[descriptor.Extension.ID]; exists {
		return fmt.Errorf("plugin registry: duplicate extension %s/%s", descriptor.PluginID, descriptor.Extension.ID)
	}
	for _, existing := range extensions {
		if existing.PackageVersion != descriptor.PackageVersion {
			return fmt.Errorf("plugin registry: inconsistent package versions for %s", descriptor.PluginID)
		}
	}
	extensions[descriptor.Extension.ID] = descriptor
	return nil
}

func (r *Registry) Lookup(pluginID ID, extensionID ExtensionID, kind Kind) (Descriptor, error) {
	if r == nil {
		return Descriptor{}, fmt.Errorf("plugin registry: nil registry")
	}
	if _, err := ParseID(pluginID.String()); err != nil {
		return Descriptor{}, err
	}
	if _, err := ParseExtensionID(extensionID.String()); err != nil {
		return Descriptor{}, err
	}
	if err := kind.Validate(); err != nil {
		return Descriptor{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if descriptor, ok := r.descriptors[pluginID][extensionID]; ok && descriptor.Extension.Kind == kind {
		return descriptor.Clone(), nil
	}
	return Descriptor{}, fmt.Errorf("plugin registry: extension not found: %s/%s (%s)", pluginID, extensionID, kind)
}

func (r *Registry) Descriptors() []Descriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Descriptor, 0)
	for _, extensions := range r.descriptors {
		for _, d := range extensions {
			result = append(result, d.Clone())
		}
	}
	slices.SortFunc(result, func(a, b Descriptor) int {
		if c := strings.Compare(a.PluginID.String(), b.PluginID.String()); c != 0 {
			return c
		}
		return strings.Compare(a.Extension.ID.String(), b.Extension.ID.String())
	})
	return result
}
