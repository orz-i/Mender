package bootstrap

import (
	"context"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	"github.com/orz-i/Mender/backend/internal/registry"
)

// SyncRegistry synchronizes the registry index metadata from the configured endpoint.
func (r *Runtime) SyncRegistry(ctx context.Context) (bool, int, error) {
	if r == nil || r.Registry == nil {
		return false, 0, fmt.Errorf("registry service unavailable")
	}
	idx, err := r.Registry.Sync(ctx, "")
	if err != nil {
		return false, 0, err
	}
	return true, len(idx.Plugins), nil
}

// InstallRegistryPlugin installs a plugin directly from the signed registry index.
func (r *Runtime) InstallRegistryPlugin(ctx context.Context, pluginID string, version string) (installationdomain.Installation, error) {
	if r == nil || r.Registry == nil {
		return installationdomain.Installation{}, fmt.Errorf("registry service unavailable")
	}
	item, err := r.Registry.Install(ctx, pluginID, version)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	item = r.postInstallProbe(ctx, item)
	return item, nil
}

// UpgradeRegistryPlugin upgrades an existing installed plugin in-place, enforcing usageCount == 0 (Option B).
func (r *Runtime) UpgradeRegistryPlugin(ctx context.Context, id platformplugin.InstallationID, version string) (installationdomain.Installation, error) {
	if r == nil || r.Registry == nil {
		return installationdomain.Installation{}, fmt.Errorf("registry service unavailable")
	}
	item, err := r.Registry.Upgrade(ctx, id, version)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	item = r.postInstallProbe(ctx, item)
	return item, nil
}

// ListRegistryUpdates checks installed plugins against the registry cache and returns available updates.
func (r *Runtime) ListRegistryUpdates(ctx context.Context) ([]registry.AvailableUpdate, error) {
	if r == nil || r.Registry == nil {
		return nil, fmt.Errorf("registry service unavailable")
	}
	installations, err := r.ListPluginInstallations(ctx)
	if err != nil {
		return nil, err
	}
	return r.Registry.CheckUpdates(ctx, installations)
}

func (r *Runtime) postInstallProbe(ctx context.Context, item installationdomain.Installation) installationdomain.Installation {
	switch item.Extension().Kind {
	case platformplugin.ProviderTools:
		if r.externalResolver != nil && r.externalProcesses != nil {
			resolved, err := r.externalResolver.ProbeInstallation(ctx, item.ID())
			if err == nil {
				platformDescriptor, err := providerplugin.PlatformDescriptor(resolved.Descriptor)
				if err == nil {
					if setErr := r.pluginInstallations.SetVerifiedDescriptor(ctx, item.ID(), platformDescriptor); setErr == nil {
						if withDesc, withErr := item.WithVerifiedDescriptor(platformDescriptor); withErr == nil {
							item = withDesc
						}
					}
				}
				r.externalResolver.ForgetInstallation(item.ID())
				_ = r.externalProcesses.CloseInstallation(item.ID())
			}
		}
	}
	return item
}

// RecordProviderUsageForTest links a test provider configuration to an installation to simulate usageCount > 0.
func (r *Runtime) RecordProviderUsageForTest(ctx context.Context, id platformplugin.InstallationID) error {
	if r == nil || r.providerConfigs == nil || r.Catalog == nil {
		return fmt.Errorf("runtime unavailable")
	}
	item, err := r.pluginInstallations.Get(ctx, id)
	if err != nil {
		return err
	}
	registeredProvider, err := r.Catalog.RegisterProvider.Execute(ctx, catalogapplication.RegisterProviderRequest{
		Namespace:   "test-usage-provider",
		DisplayName: "Test Usage Provider",
	})
	if err != nil {
		return err
	}
	desc := platformplugin.Descriptor{
		PluginID:       item.PluginID(),
		PackageVersion: item.PackageVersion(),
		Extension:      item.Extension(),
		DisplayName:    "Test",
		Description:    "Test",
		ConfigVersion:  1,
	}
	if err := r.pluginInstallations.SetVerifiedDescriptor(ctx, id, desc); err != nil {
		return err
	}
	return r.providerConfigs.Save(ctx, providerplugin.Configuration{
		ProviderID:     registeredProvider.ID(),
		PluginID:       providerplugin.ID(item.PluginID()),
		InstallationID: &id,
		ConfigVersion:  1,
		Revision:       "rev_1",
		Config:         []byte("{}"),
	})
}

type runtimePluginUsageChecker struct {
	providerConfigs *providerconfigstore.Store
	sandboxes       *Sandboxes
	tunnels         *Tunnels
}

func (c runtimePluginUsageChecker) Usages(
	ctx context.Context,
	id installationdomain.InstallationID,
) ([]installationapplication.Usage, error) {
	usages := make([]installationapplication.Usage, 0)
	if c.providerConfigs != nil {
		configs, err := c.providerConfigs.List(ctx)
		if err == nil {
			for _, cfg := range configs {
				if cfg.InstallationID != nil && *cfg.InstallationID == id {
					usages = append(usages, installationapplication.Usage{
						Kind:        "provider",
						Description: fmt.Sprintf("provider %s", cfg.ProviderID),
					})
				}
			}
		}
	}
	if c.sandboxes != nil {
		sandboxes, err := c.sandboxes.List(ctx)
		if err == nil {
			for _, s := range sandboxes {
				if s.Spec().Binding.InstallationID == id {
					usages = append(usages, installationapplication.Usage{
						Kind:        "sandbox",
						Description: fmt.Sprintf("sandbox %s", s.ID()),
					})
				}
			}
		}
	}
	if c.tunnels != nil {
		tunnels, err := c.tunnels.List.Execute(ctx)
		if err == nil {
			for _, t := range tunnels {
				if t.Binding().InstallationID == id {
					usages = append(usages, installationapplication.Usage{
						Kind:        "tunnel",
						Description: fmt.Sprintf("tunnel %s", t.ID()),
					})
				}
			}
		}
	}
	return usages, nil
}
