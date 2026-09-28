package providerplugin

import (
	"context"
	"errors"
	"fmt"
	"sync"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

var (
	// ErrManagerClosed indicates Provider activation was requested after Runtime
	// instance ownership began shutdown.
	ErrManagerClosed = errors.New("provider instance manager is closed")

	// ErrUnsupportedConfigVersion indicates persisted plugin configuration does
	// not match the registered plugin's supported version.
	ErrUnsupportedConfigVersion = errors.New("provider plugin config version is unsupported")
)

// ConfigurationSource loads the persisted plugin configuration for one Provider.
type ConfigurationSource interface {
	GetProviderConfig(context.Context, catalogdomain.ProviderID) (Configuration, error)
}

// ExecutorRoutes owns the current canonical ProviderID -> ToolExecutor route.
type ExecutorRoutes interface {
	Register(catalogdomain.ProviderID, invocationapplication.ToolExecutor) error
	Unregister(catalogdomain.ProviderID) error
}

// Manager owns active Provider plugin instances for one Runtime.
type Manager struct {
	resolver    Resolver
	configs     ConfigurationSource
	routes      ExecutorRoutes
	credentials invocationapplication.CredentialResolver

	mu     sync.Mutex
	closed bool
	slots  map[catalogdomain.ProviderID]*instanceSlot
}

type instanceSlot struct {
	mu     sync.Mutex
	active *activeInstance
}

type activeInstance struct {
	pluginID       ID
	installationID *InstallationID
	configVersion  int
	revision       string
	instance       Instance
}

// NewManager constructs Provider instance lifecycle ownership.
func NewManager(
	resolver Resolver,
	configs ConfigurationSource,
	routes ExecutorRoutes,
	credentials invocationapplication.CredentialResolver,
) (*Manager, error) {
	if resolver == nil {
		return nil, fmt.Errorf("new provider instance manager: resolver must not be nil")
	}
	if configs == nil {
		return nil, fmt.Errorf("new provider instance manager: configuration source must not be nil")
	}
	if routes == nil {
		return nil, fmt.Errorf("new provider instance manager: executor routes must not be nil")
	}
	return &Manager{
		resolver:    resolver,
		configs:     configs,
		routes:      routes,
		credentials: credentials,
		slots:       make(map[catalogdomain.ProviderID]*instanceSlot),
	}, nil
}

// Ensure returns a reusable active instance for the Provider's current
// configuration revision.
func (m *Manager) Ensure(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (Instance, error) {
	return m.ensure(ctx, providerID, false)
}

// Reload opens and installs a fresh instance even when the persisted
// configuration revision is unchanged.
func (m *Manager) Reload(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (Instance, error) {
	return m.ensure(ctx, providerID, true)
}

func (m *Manager) ensure(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	force bool,
) (Instance, error) {
	if _, err := catalogdomain.ParseProviderID(providerID.String()); err != nil {
		return nil, fmt.Errorf("ensure provider instance: %w", err)
	}

	slot, err := m.slot(providerID)
	if err != nil {
		return nil, err
	}

	slot.mu.Lock()
	defer slot.mu.Unlock()

	config, err := m.configs.GetProviderConfig(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("ensure provider instance: load config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("ensure provider instance: %w", err)
	}
	if config.ProviderID != providerID {
		return nil, fmt.Errorf(
			"ensure provider instance: configuration provider %s does not match requested provider %s",
			config.ProviderID,
			providerID,
		)
	}

	resolved, err := m.resolver.Resolve(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("ensure provider instance: resolve plugin: %w", err)
	}
	if resolved.Plugin == nil {
		return nil, fmt.Errorf("ensure provider instance: resolver returned nil plugin")
	}
	if err := resolved.Descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("ensure provider instance: descriptor: %w", err)
	}
	if resolved.Descriptor.ID != config.PluginID {
		return nil, fmt.Errorf(
			"ensure provider instance: resolved plugin id %s does not match config %s",
			resolved.Descriptor.ID,
			config.PluginID,
		)
	}
	if config.ConfigVersion != resolved.Descriptor.ConfigVersion {
		return nil, fmt.Errorf(
			"%w: plugin %s supports version %d, config is version %d",
			ErrUnsupportedConfigVersion,
			config.PluginID,
			resolved.Descriptor.ConfigVersion,
			config.ConfigVersion,
		)
	}

	if !force && sameActiveConfig(slot.active, config) {
		return slot.active.instance, nil
	}

	if err := resolved.Plugin.ValidateConfig(ctx, cloneConfig(config.Config)); err != nil {
		return nil, fmt.Errorf("ensure provider instance: validate plugin config: %w", err)
	}

	instance, err := resolved.Plugin.Open(ctx, OpenRequest{
		ProviderID:    providerID,
		ConfigVersion: config.ConfigVersion,
		Config:        cloneConfig(config.Config),
		Credentials:   m.credentials,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure provider instance: open plugin %s: %w", config.PluginID, err)
	}
	if instance == nil {
		return nil, fmt.Errorf("ensure provider instance: plugin %s returned a nil instance", config.PluginID)
	}

	if err := m.routes.Register(providerID, instance); err != nil {
		closeErr := instance.Close()
		if closeErr != nil {
			return nil, errors.Join(
				fmt.Errorf("ensure provider instance: register executor: %w", err),
				fmt.Errorf("ensure provider instance: close rejected instance: %w", closeErr),
			)
		}
		return nil, fmt.Errorf("ensure provider instance: register executor: %w", err)
	}

	previous := slot.active
	slot.active = &activeInstance{
		pluginID:       config.PluginID,
		installationID: cloneInstallationID(config.InstallationID),
		configVersion:  config.ConfigVersion,
		revision:       config.Revision,
		instance:       instance,
	}

	if previous != nil {
		if err := previous.instance.Close(); err != nil {
			return instance, fmt.Errorf(
				"ensure provider instance: close superseded instance: %w",
				err,
			)
		}
	}
	return instance, nil
}

func sameActiveConfig(active *activeInstance, config Configuration) bool {
	return active != nil &&
		active.pluginID == config.PluginID &&
		sameInstallationID(active.installationID, config.InstallationID) &&
		active.configVersion == config.ConfigVersion &&
		active.revision == config.Revision
}

func sameInstallationID(a, b *InstallationID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (m *Manager) slot(providerID catalogdomain.ProviderID) (*instanceSlot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrManagerClosed
	}
	slot := m.slots[providerID]
	if slot == nil {
		slot = &instanceSlot{}
		m.slots[providerID] = slot
	}
	return slot, nil
}

// CloseProvider removes the executor route and closes the active instance, if
// any. The Provider can be activated again later unless CloseAll has begun.
func (m *Manager) CloseProvider(providerID catalogdomain.ProviderID) error {
	if _, err := catalogdomain.ParseProviderID(providerID.String()); err != nil {
		return fmt.Errorf("close provider instance: %w", err)
	}

	m.mu.Lock()
	slot := m.slots[providerID]
	m.mu.Unlock()
	if slot == nil {
		return nil
	}

	slot.mu.Lock()
	defer slot.mu.Unlock()

	if slot.active == nil {
		return nil
	}
	if err := m.routes.Unregister(providerID); err != nil {
		return fmt.Errorf("close provider instance: unregister executor: %w", err)
	}

	instance := slot.active.instance
	slot.active = nil
	if err := instance.Close(); err != nil {
		return fmt.Errorf("close provider instance: %w", err)
	}
	return nil
}

// CloseAll permanently closes the manager and all active Provider instances.
func (m *Manager) CloseAll() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	type slotEntry struct {
		providerID catalogdomain.ProviderID
		slot       *instanceSlot
	}
	entries := make([]slotEntry, 0, len(m.slots))
	for providerID, slot := range m.slots {
		entries = append(entries, slotEntry{providerID: providerID, slot: slot})
	}
	m.mu.Unlock()

	var errs []error
	for _, entry := range entries {
		entry.slot.mu.Lock()
		if entry.slot.active != nil {
			if err := m.routes.Unregister(entry.providerID); err != nil {
				errs = append(errs, fmt.Errorf(
					"close provider instance %s: unregister executor: %w",
					entry.providerID,
					err,
				))
			}
			if err := entry.slot.active.instance.Close(); err != nil {
				errs = append(errs, fmt.Errorf(
					"close provider instance %s: %w",
					entry.providerID,
					err,
				))
			}
			entry.slot.active = nil
		}
		entry.slot.mu.Unlock()
	}
	return errors.Join(errs...)
}
