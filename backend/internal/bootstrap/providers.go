package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

// ProviderPluginDescriptors returns safe metadata for all built-in Provider
// plugins available to this Runtime.
func (r *Runtime) ProviderPluginDescriptors() []providerplugin.Descriptor {
	if r == nil || r.providerPlugins == nil {
		return nil
	}
	return r.providerPlugins.Descriptors()
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
	plugin, descriptor, err := r.resolveProviderPlugin(currentConfig.PluginID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: %w",
			err,
		)
	}
	if err := plugin.ValidateConfig(ctx, cloneRawJSON(rawConfig)); err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: validate config: %w",
			err,
		)
	}

	nextConfig, err := providerplugin.NewConfiguration(
		providerID,
		currentConfig.PluginID,
		descriptor.ConfigVersion,
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
