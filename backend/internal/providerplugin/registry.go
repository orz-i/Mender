package providerplugin

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

var (
	// ErrNotFound indicates no registered Provider plugin owns the requested ID.
	ErrNotFound = errors.New("provider plugin not found")

	// ErrDuplicate indicates a Provider plugin ID was registered more than once.
	ErrDuplicate = errors.New("provider plugin already registered")
)

// Registry owns the built-in Provider plugins available to one Runtime.
type Registry struct {
	mu      sync.RWMutex
	plugins map[ID]registeredPlugin
}

type registeredPlugin struct {
	plugin     Plugin
	descriptor Descriptor
}

// NewRegistry constructs an empty Provider plugin registry.
func NewRegistry() *Registry {
	return &Registry{plugins: make(map[ID]registeredPlugin)}
}

// Register adds one Provider plugin.
//
// Registration validates and snapshots Descriptor metadata so later management
// reads do not invoke plugin descriptor logic repeatedly.
func (r *Registry) Register(plugin Plugin) error {
	if r == nil {
		return fmt.Errorf("register provider plugin: registry must not be nil")
	}
	if plugin == nil {
		return fmt.Errorf("register provider plugin: plugin must not be nil")
	}

	descriptor := plugin.Descriptor()
	if err := descriptor.Validate(); err != nil {
		return fmt.Errorf("register provider plugin: %w", err)
	}
	descriptor.Version = strings.TrimSpace(descriptor.Version)
	descriptor.DisplayName = strings.TrimSpace(descriptor.DisplayName)
	descriptor.Description = strings.TrimSpace(descriptor.Description)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[descriptor.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicate, descriptor.ID)
	}
	r.plugins[descriptor.ID] = registeredPlugin{
		plugin:     plugin,
		descriptor: descriptor,
	}
	return nil
}

// Lookup resolves a registered Provider plugin.
func (r *Registry) Lookup(id ID) (Plugin, error) {
	entry, err := r.lookup(id)
	if err != nil {
		return nil, err
	}
	return entry.plugin, nil
}

func (r *Registry) lookup(id ID) (registeredPlugin, error) {
	if r == nil {
		return registeredPlugin{}, fmt.Errorf("lookup provider plugin: registry must not be nil")
	}
	if _, err := ParseID(id.String()); err != nil {
		return registeredPlugin{}, fmt.Errorf("lookup provider plugin: %w", err)
	}

	r.mu.RLock()
	entry, ok := r.plugins[id]
	r.mu.RUnlock()
	if !ok {
		return registeredPlugin{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return entry, nil
}

// Descriptors returns stable management-safe metadata ordered by PluginID.
func (r *Registry) Descriptors() []Descriptor {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	descriptors := make([]Descriptor, 0, len(r.plugins))
	for _, entry := range r.plugins {
		descriptors = append(descriptors, entry.descriptor)
	}
	r.mu.RUnlock()

	slices.SortFunc(descriptors, func(a, b Descriptor) int {
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	return descriptors
}
