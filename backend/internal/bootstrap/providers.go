package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
)

const (
	ProviderPluginSourceBuiltIn  = "builtin"
	ProviderPluginSourceExternal = "external"
)

// AvailableProviderPlugin is management-safe resolution metadata for one
// selectable Provider plugin implementation.
type AvailableProviderPlugin struct {
	Descriptor     providerplugin.Descriptor
	SourceKind     string
	InstallationID *providerplugin.InstallationID
	Enabled        bool
}

// ProviderPluginDescriptors returns safe metadata for all built-in Provider
// plugins available to this Runtime.
func (r *Runtime) ProviderPluginDescriptors() []providerplugin.Descriptor {
	if r == nil || r.providerPlugins == nil {
		return nil
	}
	return r.providerPlugins.Descriptors()
}

// ListAvailableProviderPlugins returns built-in plugins plus enabled external
// installations, preserving exact installation identity for side-by-side
// package versions.
func (r *Runtime) ListAvailableProviderPlugins(
	ctx context.Context,
) ([]AvailableProviderPlugin, error) {
	if r == nil {
		return nil, fmt.Errorf("list available provider plugins: runtime is nil")
	}

	builtins := r.providerPlugins.Descriptors()
	result := make([]AvailableProviderPlugin, 0, len(builtins))
	for _, descriptor := range builtins {
		result = append(result, AvailableProviderPlugin{
			Descriptor: descriptor.Clone(),
			SourceKind: ProviderPluginSourceBuiltIn,
			Enabled:    true,
		})
	}

	installations, err := r.ListProviderPluginInstallations(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"list available provider plugins: installations: %w",
			err,
		)
	}
	for _, installation := range installations {
		if installation.Status() != installationdomain.StatusEnabled {
			continue
		}
		resolved, resolveErr := r.externalResolver.ResolveInstallation(
			ctx,
			installation.ID(),
		)
		if resolveErr != nil {
			return nil, fmt.Errorf(
				"list available provider plugins: installation %s: %w",
				installation.ID(),
				resolveErr,
			)
		}
		id := installation.ID()
		result = append(result, AvailableProviderPlugin{
			Descriptor:     resolved.Descriptor.Clone(),
			SourceKind:     ProviderPluginSourceExternal,
			InstallationID: &id,
			Enabled:        true,
		})
	}

	slices.SortFunc(result, func(a, b AvailableProviderPlugin) int {
		if comparison := strings.Compare(
			a.Descriptor.ID.String(),
			b.Descriptor.ID.String(),
		); comparison != 0 {
			return comparison
		}
		if comparison := strings.Compare(a.SourceKind, b.SourceKind); comparison != 0 {
			return comparison
		}
		var aInstallation, bInstallation string
		if a.InstallationID != nil {
			aInstallation = a.InstallationID.String()
		}
		if b.InstallationID != nil {
			bInstallation = b.InstallationID.String()
		}
		return strings.Compare(aInstallation, bInstallation)
	})
	return result, nil
}

// ProviderConfig returns the generic plugin configuration for one Provider.
func (r *Runtime) ProviderConfig(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (providerplugin.Configuration, error) {
	return r.providerConfigs.GetProviderConfig(ctx, providerID)
}

// ListProviderConfigs returns all generic Provider plugin configurations.
func (r *Runtime) ListProviderConfigs(
	ctx context.Context,
) ([]providerplugin.Configuration, error) {
	return r.providerConfigs.List(ctx)
}

// RegisterConfiguredProvider persists canonical Provider metadata and a
// plugin-owned configuration. Configuration persistence failure is compensated
// by removing the newly-created canonical Provider.
func (r *Runtime) RegisterConfiguredProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	plugin, descriptor, err := r.resolveProviderPlugin(pluginID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register configured provider: %w",
			err,
		)
	}
	if err := plugin.ValidateConfig(ctx, cloneRawJSON(rawConfig)); err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register configured provider: validate config: %w",
			err,
		)
	}

	provider, err := r.Catalog.RegisterProvider.Execute(ctx, request)
	if err != nil {
		return catalogdomain.Provider{}, err
	}

	config, err := providerplugin.NewConfiguration(
		provider.ID(),
		descriptor.ID,
		descriptor.ConfigVersion,
		rawConfig,
	)
	if err != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("register configured provider: build config: %w", err),
			wrapOptionalError(
				"register configured provider: compensate provider removal",
				removeErr,
			),
		)
	}
	if err := r.providerConfigs.Save(ctx, config); err != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("register configured provider: save config: %w", err),
			wrapOptionalError(
				"register configured provider: compensate provider removal",
				removeErr,
			),
		)
	}
	return provider, nil
}

// RegisterInstalledProvider persists a Provider pinned to one exact external
// plugin installation. It is intentionally an internal Runtime primitive until
// management adapters expose installation-aware creation.
func (r *Runtime) RegisterInstalledProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	if r == nil || r.externalResolver == nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register installed provider: external plugin resolver is unavailable",
		)
	}
	resolved, err := r.externalResolver.ResolveInstallation(ctx, installationID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register installed provider: %w",
			err,
		)
	}
	if resolved.Descriptor.ID != pluginID {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register installed provider: installation %s owns plugin %s, request uses %s",
			installationID,
			resolved.Descriptor.ID,
			pluginID,
		)
	}
	if err := resolved.Plugin.ValidateConfig(ctx, cloneRawJSON(rawConfig)); err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register installed provider: validate config: %w",
			err,
		)
	}

	provider, err := r.Catalog.RegisterProvider.Execute(ctx, request)
	if err != nil {
		return catalogdomain.Provider{}, err
	}

	config, err := providerplugin.NewInstalledConfiguration(
		provider.ID(),
		resolved.Descriptor.ID,
		installationID,
		resolved.Descriptor.ConfigVersion,
		rawConfig,
	)
	if err != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("register installed provider: build config: %w", err),
			wrapOptionalError(
				"register installed provider: compensate provider removal",
				removeErr,
			),
		)
	}
	if err := r.providerConfigs.Save(ctx, config); err != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("register installed provider: save config: %w", err),
			wrapOptionalError(
				"register installed provider: compensate provider removal",
				removeErr,
			),
		)
	}
	return provider, nil
}

// UpdateConfiguredProvider updates canonical metadata and plugin configuration
// while preserving Provider identity and PluginID.
func (r *Runtime) UpdateConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	request catalogapplication.UpdateProviderRequest,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	currentProvider, err := r.Catalog.GetProvider.Execute(ctx, providerID)
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	if _, err := currentProvider.UpdateMetadata(
		request.DisplayName,
		request.Description,
		r.Catalog.clock.Now(),
	); err != nil {
		return catalogdomain.Provider{}, err
	}

	currentConfig, err := r.providerConfigs.GetProviderConfig(ctx, providerID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: load config: %w",
			err,
		)
	}
	resolved, err := r.providerResolver.Resolve(ctx, currentConfig)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: resolve plugin: %w",
			err,
		)
	}
	if err := resolved.Plugin.ValidateConfig(ctx, cloneRawJSON(rawConfig)); err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: validate config: %w",
			err,
		)
	}

	nextConfig, err := updatedProviderConfiguration(
		providerID,
		currentConfig,
		resolved.Descriptor.ConfigVersion,
		rawConfig,
	)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: build config: %w",
			err,
		)
	}
	if err := r.providerConfigs.Save(ctx, nextConfig); err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: save config: %w",
			err,
		)
	}

	request.ProviderID = providerID
	updated, err := r.Catalog.UpdateProvider.Execute(ctx, request)
	if err != nil {
		restoreErr := r.providerConfigs.Save(ctx, currentConfig)
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("update configured provider: metadata: %w", err),
			wrapOptionalError(
				"update configured provider: restore config",
				restoreErr,
			),
		)
	}
	return updated, nil
}

// SyncConfiguredProvider reloads the current Provider plugin instance and
// atomically refreshes its canonical Tool snapshot.
func (r *Runtime) SyncConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (catalogapplication.SyncProviderToolsResult, error) {
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"sync configured provider: provider: %w",
			err,
		)
	}

	instance, err := r.providerInstances.Reload(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"sync configured provider: activate: %w",
			err,
		)
	}
	syncTools := r.Catalog.SyncProviderTools(instance)
	result, err := syncTools.Execute(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"sync configured provider: synchronize: %w",
			err,
		)
	}
	return result, nil
}

// EnsureConfiguredProvider makes the Provider's current plugin instance
// available for execution without refreshing Catalog metadata.
func (r *Runtime) EnsureConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		return fmt.Errorf("ensure configured provider: provider: %w", err)
	}
	if _, err := r.providerInstances.Ensure(ctx, providerID); err != nil {
		return fmt.Errorf("ensure configured provider: %w", err)
	}
	return nil
}

// DeleteConfiguredProvider removes generic Provider configuration and canonical
// Provider state after checking Profile/Policy references.
func (r *Runtime) DeleteConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	if err := r.ensureProviderUnused(ctx, providerID); err != nil {
		return err
	}

	config, err := r.providerConfigs.GetProviderConfig(ctx, providerID)
	if err != nil && !errors.Is(err, providerconfigstore.ErrNotFound) {
		return fmt.Errorf("delete configured provider: load config: %w", err)
	}
	hadConfig := err == nil

	if err := r.providerInstances.CloseProvider(providerID); err != nil {
		return fmt.Errorf("delete configured provider: close active instance: %w", err)
	}

	if hadConfig {
		if err := r.providerConfigs.Delete(ctx, providerID); err != nil {
			return fmt.Errorf("delete configured provider: delete config: %w", err)
		}
	}
	if err := r.Catalog.RemoveProvider.Execute(ctx, providerID); err != nil {
		var restoreErr error
		if hadConfig {
			restoreErr = r.providerConfigs.Save(ctx, config)
		}
		return errors.Join(
			fmt.Errorf("delete configured provider: remove provider: %w", err),
			wrapOptionalError(
				"delete configured provider: restore config",
				restoreErr,
			),
		)
	}
	return nil
}

func (r *Runtime) ensureProviderUnused(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	profiles, err := r.Profiles.List.Execute(ctx)
	if err != nil {
		return fmt.Errorf("check provider usage: list profiles: %w", err)
	}
	for _, profile := range profiles {
		for _, selection := range profile.Selections() {
			tool, describeErr := r.Catalog.DescribeTool.Execute(ctx, selection.ToolID)
			if describeErr != nil {
				if errors.Is(describeErr, catalogapplication.ErrToolNotFound) {
					continue
				}
				return fmt.Errorf(
					"check provider usage: inspect profile %s: %w",
					profile.Slug(),
					describeErr,
				)
			}
			if tool.ProviderID() == providerID {
				return fmt.Errorf(
					"%w: profile %s selects tool %s",
					ErrProviderInUse,
					profile.Slug(),
					tool.ID(),
				)
			}
		}
	}

	policy, err := r.Policy.Get.Execute(ctx)
	if err != nil {
		return fmt.Errorf("check provider usage: inspect Policy: %w", err)
	}
	for _, rule := range policy.Rules() {
		for _, referencedProviderID := range rule.Match().ProviderIDs() {
			if referencedProviderID == providerID.String() {
				return fmt.Errorf(
					"%w: Policy Rule %s references Provider %s",
					ErrProviderInUse,
					rule.ID(),
					providerID,
				)
			}
		}
	}
	return nil
}

func updatedProviderConfiguration(
	providerID catalogdomain.ProviderID,
	current providerplugin.Configuration,
	configVersion int,
	rawConfig json.RawMessage,
) (providerplugin.Configuration, error) {
	if current.InstallationID == nil {
		return providerplugin.NewConfiguration(
			providerID,
			current.PluginID,
			configVersion,
			rawConfig,
		)
	}
	return providerplugin.NewInstalledConfiguration(
		providerID,
		current.PluginID,
		*current.InstallationID,
		configVersion,
		rawConfig,
	)
}

func (r *Runtime) resolveProviderPlugin(
	pluginID providerplugin.ID,
) (providerplugin.Plugin, providerplugin.Descriptor, error) {
	plugin, err := r.providerPlugins.Lookup(pluginID)
	if err != nil {
		return nil, providerplugin.Descriptor{}, err
	}
	descriptor := plugin.Descriptor()
	if descriptor.ID != pluginID {
		return nil, providerplugin.Descriptor{}, fmt.Errorf(
			"provider plugin descriptor id %s does not match registry id %s",
			descriptor.ID,
			pluginID,
		)
	}
	return plugin, descriptor, nil
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
