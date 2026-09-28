package providerplugin

import (
	"context"
	"fmt"
)

// Resolution is one resolved Provider plugin implementation plus its validated descriptor.
type Resolution struct {
	Plugin     Plugin
	Descriptor Descriptor
}

// Resolver resolves the exact plugin implementation bound by a Provider configuration.
type Resolver interface {
	Resolve(context.Context, Configuration) (Resolution, error)
}

// Resolve implements Resolver for built-in plugin configurations.
func (r *Registry) Resolve(
	_ context.Context,
	config Configuration,
) (Resolution, error) {
	if config.InstallationID != nil {
		return Resolution{}, fmt.Errorf(
			"resolve built-in provider plugin: configuration %s is pinned to installation %s",
			config.ProviderID,
			config.InstallationID,
		)
	}
	entry, err := r.lookup(config.PluginID)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{
		Plugin:     entry.plugin,
		Descriptor: entry.descriptor,
	}, nil
}

var _ Resolver = (*Registry)(nil)
