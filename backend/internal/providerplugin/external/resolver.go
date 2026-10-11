package external

import (
	"context"
	"fmt"
	"sync"

	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// InstallationSource loads exact installed external plugin packages by stable identity.
type InstallationSource interface {
	Get(
		context.Context,
		installationdomain.InstallationID,
	) (installationdomain.Installation, error)
}

type cachedPlugin struct {
	digest string
	plugin *Plugin
}

// Resolver composes built-in plugin lookup with exact installed external packages.
type Resolver struct {
	builtins      *providerplugin.Registry
	installations InstallationSource
	processes     processProvider

	mu    sync.RWMutex
	cache map[providerplugin.InstallationID]cachedPlugin
}

// NewResolver constructs a built-in/external Provider plugin resolver.
func NewResolver(
	builtins *providerplugin.Registry,
	installations InstallationSource,
	processes processProvider,
) (*Resolver, error) {
	if builtins == nil {
		return nil, fmt.Errorf("new provider plugin resolver: built-in registry must not be nil")
	}
	if installations == nil {
		return nil, fmt.Errorf("new provider plugin resolver: installation source must not be nil")
	}
	if processes == nil {
		return nil, fmt.Errorf("new provider plugin resolver: process provider must not be nil")
	}
	return &Resolver{
		builtins:      builtins,
		installations: installations,
		processes:     processes,
		cache:         make(map[providerplugin.InstallationID]cachedPlugin),
	}, nil
}

// Resolve implements providerplugin.Resolver.
func (r *Resolver) Resolve(
	ctx context.Context,
	config providerplugin.Configuration,
) (providerplugin.Resolution, error) {
	if err := config.Validate(); err != nil {
		return providerplugin.Resolution{}, fmt.Errorf(
			"resolve provider plugin: configuration: %w",
			err,
		)
	}
	if config.InstallationID == nil {
		return r.builtins.Resolve(ctx, config)
	}

	resolved, err := r.ResolveInstallation(ctx, *config.InstallationID)
	if err != nil {
		return providerplugin.Resolution{}, err
	}
	if resolved.Descriptor.ID != config.PluginID {
		return providerplugin.Resolution{}, fmt.Errorf(
			"resolve provider plugin: installation %s owns plugin %s, config requests %s",
			config.InstallationID.String(),
			resolved.Descriptor.ID,
			config.PluginID,
		)
	}
	return resolved, nil
}

// ForgetInstallation drops the cached runtime adapter for one installation.
// The package process lifecycle remains owned by Supervisor.
func (r *Resolver) ForgetInstallation(id providerplugin.InstallationID) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.cache, id)
	r.mu.Unlock()
}

// ProbeInstallation explicitly executes one newly installed package to verify
// and snapshot its descriptor before any ProviderID exists. It is installation
// trust verification, not Provider activation, and must not be used by normal
// runtime resolution or management discovery.
func (r *Resolver) ProbeInstallation(
	ctx context.Context,
	id providerplugin.InstallationID,
) (providerplugin.Resolution, error) {
	installation, err := r.installations.Get(ctx, id)
	if err != nil {
		return providerplugin.Resolution{}, fmt.Errorf(
			"probe provider plugin: installation %s: %w",
			id,
			err,
		)
	}
	if err := installation.Validate(); err != nil {
		return providerplugin.Resolution{}, fmt.Errorf(
			"probe provider plugin: installation %s: %w",
			id,
			err,
		)
	}
	if installation.Status() != installationdomain.StatusEnabled {
		return providerplugin.Resolution{}, fmt.Errorf(
			"probe provider plugin: installation %s is %s",
			installation.ID(),
			installation.Status(),
		)
	}
	plugin, err := NewPlugin(ctx, installation, r.processes)
	if err != nil {
		return providerplugin.Resolution{}, fmt.Errorf(
			"probe provider plugin: installation %s: %w",
			installation.ID(),
			err,
		)
	}
	return providerplugin.Resolution{
		Plugin:     plugin,
		Descriptor: plugin.Descriptor(),
	}, nil
}

// ResolveInstallation resolves one exact enabled external plugin package from
// its immutable verified descriptor snapshot without starting plugin code.
func (r *Resolver) ResolveInstallation(
	ctx context.Context,
	id providerplugin.InstallationID,
) (providerplugin.Resolution, error) {
	installation, err := r.installations.Get(ctx, id)
	if err != nil {
		return providerplugin.Resolution{}, fmt.Errorf(
			"resolve provider plugin: installation %s: %w",
			id,
			err,
		)
	}
	if err := installation.Validate(); err != nil {
		return providerplugin.Resolution{}, fmt.Errorf(
			"resolve provider plugin: installation %s: %w",
			id,
			err,
		)
	}
	if installation.Status() != installationdomain.StatusEnabled {
		return providerplugin.Resolution{}, fmt.Errorf(
			"resolve provider plugin: installation %s is %s",
			installation.ID(),
			installation.Status(),
		)
	}

	r.mu.RLock()
	cached, ok := r.cache[id]
	r.mu.RUnlock()
	if ok {
		if cached.digest != installation.Digest() {
			return providerplugin.Resolution{}, fmt.Errorf(
				"resolve provider plugin: installation %s digest changed",
				installation.ID(),
			)
		}
		return providerplugin.Resolution{
			Plugin:     cached.plugin,
			Descriptor: cached.plugin.Descriptor(),
		}, nil
	}

	plugin, err := NewPluginFromVerifiedDescriptor(installation, r.processes)
	if err != nil {
		return providerplugin.Resolution{}, fmt.Errorf(
			"resolve provider plugin: installation %s: %w",
			installation.ID(),
			err,
		)
	}
	descriptor := plugin.Descriptor()
	if descriptor.ID != installation.PluginID() {
		return providerplugin.Resolution{}, fmt.Errorf(
			"resolve provider plugin: descriptor id %s does not match installation %s",
			descriptor.ID,
			installation.PluginID(),
		)
	}

	r.mu.Lock()
	existing, exists := r.cache[id]
	if exists {
		r.mu.Unlock()
		if existing.digest != installation.Digest() {
			return providerplugin.Resolution{}, fmt.Errorf(
				"resolve provider plugin: installation %s digest changed",
				installation.ID(),
			)
		}
		return providerplugin.Resolution{
			Plugin:     existing.plugin,
			Descriptor: existing.plugin.Descriptor(),
		}, nil
	}
	r.cache[id] = cachedPlugin{
		digest: installation.Digest(),
		plugin: plugin,
	}
	r.mu.Unlock()

	return providerplugin.Resolution{
		Plugin:     plugin,
		Descriptor: descriptor,
	}, nil
}

var _ providerplugin.Resolver = (*Resolver)(nil)
