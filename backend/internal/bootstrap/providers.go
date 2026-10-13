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
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
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

// ProviderPluginIssue is one installation-scoped catalog degradation that does
// not hide unrelated healthy Provider plugins.
type ProviderPluginIssue struct {
	InstallationID providerplugin.InstallationID
	Code           string
	Message        string
}

// ProviderPluginCatalog is the complete management projection of selectable
// plugins plus installation-scoped degradation diagnostics.
type ProviderPluginCatalog struct {
	Plugins []AvailableProviderPlugin
	Issues  []ProviderPluginIssue
}

const (
	providerPluginIssueResolveDescriptor = "descriptor_resolution_failed"
	providerPluginIssuePersistDescriptor = "descriptor_backfill_failed"
	providerPluginIssueCloseProcess      = "process_cleanup_failed"
)

// ProviderPluginDescriptors returns only user-configurable Provider
// integrations. The Jobs Provider is a private host-owned Tool Suite facade,
// never a generic Provider offering in the plugin marketplace.
func (r *Runtime) ProviderPluginDescriptors() []providerplugin.Descriptor {
	if r == nil || r.providerPlugins == nil {
		return nil
	}
	return slices.DeleteFunc(r.providerPlugins.Descriptors(), func(d providerplugin.Descriptor) bool {
		return d.ID == sshjobsplugin.ID
	})
}

// ProviderPluginDescriptorForConfig returns management-safe descriptor metadata
// for the exact plugin implementation bound by one persisted Provider config.
// External descriptors are read from the verified installation snapshot and do
// not start plugin processes.
func (r *Runtime) ProviderPluginDescriptorForConfig(
	ctx context.Context,
	config providerplugin.Configuration,
) (providerplugin.Descriptor, bool, error) {
	if r == nil {
		return providerplugin.Descriptor{}, false, fmt.Errorf(
			"provider plugin descriptor for config: runtime is nil",
		)
	}
	if config.InstallationID == nil {
		// Exact persisted, already-authorized facades must remain resolvable
		// even when their private plugin is absent from the public catalogue.
		descriptor, err := r.providerPlugins.Lookup(config.PluginID)
		if err != nil {
			return providerplugin.Descriptor{}, false, nil
		}
		return descriptor.Descriptor().Clone(), true, nil
	}

	installation, err := r.pluginInstallations.Get(ctx, *config.InstallationID)
	if err != nil {
		return providerplugin.Descriptor{}, false, fmt.Errorf(
			"provider plugin descriptor for config: installation %s: %w",
			config.InstallationID,
			err,
		)
	}
	if installation.PluginID() != config.PluginID {
		return providerplugin.Descriptor{}, false, fmt.Errorf(
			"provider plugin descriptor for config: installation %s owns plugin %s, config uses %s",
			installation.ID(),
			installation.PluginID(),
			config.PluginID,
		)
	}
	descriptor, ok := installation.VerifiedDescriptor()
	if !ok {
		return providerplugin.Descriptor{}, false, nil
	}
	typed, err := providerplugin.ProviderDescriptor(descriptor)
	if err != nil {
		return providerplugin.Descriptor{}, false, err
	}
	return typed, true, nil
}

// ListAvailableProviderPluginCatalog returns built-in plugins plus enabled
// external installations and preserves installation-scoped backfill failures as
// diagnostics instead of hiding unrelated healthy plugins.
func (r *Runtime) ListAvailableProviderPluginCatalog(
	ctx context.Context,
) (ProviderPluginCatalog, error) {
	if r == nil {
		return ProviderPluginCatalog{}, fmt.Errorf(
			"list available provider plugins: runtime is nil",
		)
	}

	builtins := r.ProviderPluginDescriptors()
	plugins := make([]AvailableProviderPlugin, 0, len(builtins))
	for _, descriptor := range builtins {
		plugins = append(plugins, AvailableProviderPlugin{
			Descriptor: descriptor.Clone(),
			SourceKind: ProviderPluginSourceBuiltIn,
			Enabled:    true,
		})
	}

	installations, err := r.ListProviderPluginInstallations(ctx)
	if err != nil {
		return ProviderPluginCatalog{}, fmt.Errorf(
			"list available provider plugins: installations: %w",
			err,
		)
	}
	issues := make([]ProviderPluginIssue, 0)
	for _, installation := range installations {
		if installation.Status() != installationdomain.StatusEnabled || installation.Extension().Kind != platformplugin.ProviderTools {
			continue
		}
		platformDescriptor, verified := installation.VerifiedDescriptor()
		var descriptor providerplugin.Descriptor
		if verified {
			descriptor, err = providerplugin.ProviderDescriptor(platformDescriptor)
			if err != nil {
				issues = append(issues, newProviderPluginIssue(installation.ID(), providerPluginIssueResolveDescriptor, err))
				continue
			}
		}
		if !verified {
			var backfillIssues []ProviderPluginIssue
			descriptor, verified, backfillIssues = backfillProviderPluginDescriptor(
				ctx,
				installation,
				r.externalResolver,
				r.pluginInstallations,
				r.externalProcesses,
			)
			issues = append(issues, backfillIssues...)
			if !verified {
				continue
			}
			r.externalResolver.ForgetInstallation(installation.ID())
		}
		id := installation.ID()
		plugins = append(plugins, AvailableProviderPlugin{
			Descriptor:     descriptor.Clone(),
			SourceKind:     ProviderPluginSourceExternal,
			InstallationID: &id,
			Enabled:        true,
		})
	}

	slices.SortFunc(plugins, func(a, b AvailableProviderPlugin) int {
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
	slices.SortFunc(issues, func(a, b ProviderPluginIssue) int {
		if comparison := strings.Compare(
			a.InstallationID.String(),
			b.InstallationID.String(),
		); comparison != 0 {
			return comparison
		}
		return strings.Compare(a.Code, b.Code)
	})
	return ProviderPluginCatalog{Plugins: plugins, Issues: issues}, nil
}

// ListAvailableProviderPlugins retains the original plugin-only Runtime API for
// callers that do not need installation-scoped catalog diagnostics.
func (r *Runtime) ListAvailableProviderPlugins(
	ctx context.Context,
) ([]AvailableProviderPlugin, error) {
	catalog, err := r.ListAvailableProviderPluginCatalog(ctx)
	if err != nil {
		return nil, err
	}
	return catalog.Plugins, nil
}

type providerPluginDescriptorBackfillResolver interface {
	ResolveInstallation(
		context.Context,
		providerplugin.InstallationID,
	) (providerplugin.Resolution, error)
}

type providerPluginVerifiedDescriptorStore interface {
	SetVerifiedDescriptor(
		context.Context,
		providerplugin.InstallationID,
		platformplugin.Descriptor,
	) error
}

type providerPluginProcessCloser interface {
	CloseInstallation(providerplugin.InstallationID) error
}

func backfillProviderPluginDescriptor(
	ctx context.Context,
	installation installationdomain.Installation,
	resolver providerPluginDescriptorBackfillResolver,
	store providerPluginVerifiedDescriptorStore,
	processes providerPluginProcessCloser,
) (providerplugin.Descriptor, bool, []ProviderPluginIssue) {
	resolved, err := resolver.ResolveInstallation(ctx, installation.ID())
	if err != nil {
		return providerplugin.Descriptor{}, false, []ProviderPluginIssue{
			newProviderPluginIssue(
				installation.ID(),
				providerPluginIssueResolveDescriptor,
				err,
			),
		}
	}
	descriptor := resolved.Descriptor.Clone()
	platformDescriptor, conversionErr := providerplugin.PlatformDescriptor(descriptor)
	if conversionErr != nil {
		return providerplugin.Descriptor{}, false, []ProviderPluginIssue{
			newProviderPluginIssue(installation.ID(), providerPluginIssuePersistDescriptor, conversionErr),
		}
	}
	persistErr := store.SetVerifiedDescriptor(
		context.WithoutCancel(ctx), installation.ID(), platformDescriptor,
	)
	closeErr := processes.CloseInstallation(installation.ID())

	issues := make([]ProviderPluginIssue, 0, 2)
	if persistErr != nil {
		issues = append(issues, newProviderPluginIssue(
			installation.ID(),
			providerPluginIssuePersistDescriptor,
			persistErr,
		))
	}
	if closeErr != nil {
		issues = append(issues, newProviderPluginIssue(
			installation.ID(),
			providerPluginIssueCloseProcess,
			closeErr,
		))
	}
	if persistErr != nil {
		return providerplugin.Descriptor{}, false, issues
	}
	return descriptor, true, issues
}

func newProviderPluginIssue(
	installationID providerplugin.InstallationID,
	code string,
	err error,
) ProviderPluginIssue {
	message := strings.TrimSpace(err.Error())
	const maxRunes = 512
	runes := []rune(message)
	if len(runes) > maxRunes {
		message = string(runes[:maxRunes]) + "…"
	}
	return ProviderPluginIssue{
		InstallationID: installationID,
		Code:           code,
		Message:        message,
	}
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
// plugin-owned configuration. Any plugin code runs only after Provider
// activation admission succeeds.
func (r *Runtime) RegisterConfiguredProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
) (catalogdomain.Provider, error) {
	return r.registerConfiguredProvider(ctx, request, pluginID, rawConfig, governanceContext, false)
}

// registerConfiguredProvider may authorize an internal Jobs Provider facade
// only from the owner-approved Tool Suite creation use case, not from
// generic Provider CRUD, CLI, HTTP, plugins or model-controlled arguments.
func (r *Runtime) registerConfiguredProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
	managedJobs bool,
) (catalogdomain.Provider, error) {
	if pluginID == sshjobsplugin.ID && !managedJobs {
		return catalogdomain.Provider{}, sandboxdomain.ErrNotAuthorized
	}
	plugin, descriptor, err := r.resolveProviderPlugin(pluginID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register configured provider: %w",
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

	activationErr := r.admitProviderActivation(
		ctx,
		provider.ID(),
		governanceContext,
		func(operationCtx context.Context) error {
			if err := plugin.ValidateConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			); err != nil {
				return fmt.Errorf(
					"register configured provider: validate config: %w",
					err,
				)
			}
			metadata, err := plugin.InspectConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			)
			if err != nil {
				return fmt.Errorf(
					"register configured provider: inspect config: %w",
					err,
				)
			}
			nextConfig, err := config.WithMetadata(metadata)
			if err != nil {
				return fmt.Errorf(
					"register configured provider: normalize config metadata: %w",
					err,
				)
			}
			if err := r.providerConfigs.Save(operationCtx, nextConfig); err != nil {
				return fmt.Errorf(
					"register configured provider: save config: %w",
					err,
				)
			}
			return nil
		},
	)
	if activationErr != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(
			context.WithoutCancel(ctx),
			provider.ID(),
		)
		return catalogdomain.Provider{}, errors.Join(
			activationErr,
			wrapOptionalError(
				"register configured provider: compensate provider removal",
				removeErr,
			),
		)
	}
	return provider, nil
}

// RegisterInstalledProvider persists a Provider pinned to one exact external
// plugin installation. Descriptor resolution is offline; any plugin process/RPC
// runs only after Provider activation admission succeeds.
func (r *Runtime) RegisterInstalledProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
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

	activationErr := r.admitProviderActivation(
		ctx,
		provider.ID(),
		governanceContext,
		func(operationCtx context.Context) error {
			if err := resolved.Plugin.ValidateConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			); err != nil {
				return fmt.Errorf(
					"register installed provider: validate config: %w",
					err,
				)
			}
			metadata, err := resolved.Plugin.InspectConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			)
			if err != nil {
				return fmt.Errorf(
					"register installed provider: inspect config: %w",
					err,
				)
			}
			nextConfig, err := config.WithMetadata(metadata)
			if err != nil {
				return fmt.Errorf(
					"register installed provider: normalize config metadata: %w",
					err,
				)
			}
			if err := r.providerConfigs.Save(operationCtx, nextConfig); err != nil {
				return fmt.Errorf(
					"register installed provider: save config: %w",
					err,
				)
			}
			return nil
		},
	)
	if activationErr != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(
			context.WithoutCancel(ctx),
			provider.ID(),
		)
		return catalogdomain.Provider{}, errors.Join(
			activationErr,
			wrapOptionalError(
				"register installed provider: compensate provider removal",
				removeErr,
			),
		)
	}
	return provider, nil
}

// UpdateConfiguredProvider updates canonical metadata and plugin configuration
// while preserving Provider identity and PluginID. Plugin semantic work is
// admitted as provider.activate before any plugin process/RPC can run.
func (r *Runtime) UpdateConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	request catalogapplication.UpdateProviderRequest,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
) (catalogdomain.Provider, error) {
	if r.Sandboxes != nil && r.Sandboxes.store != nil {
		managed, err := r.Sandboxes.store.IsManagedSandboxSuiteProvider(ctx, providerID.String())
		if err != nil {
			return catalogdomain.Provider{}, err
		}
		if managed {
			return catalogdomain.Provider{}, fmt.Errorf("%w: Provider belongs to a Sandbox Tool Suite", ErrProviderInUse)
		}
	}
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

	var updated catalogdomain.Provider
	activationErr := r.admitProviderActivation(
		ctx,
		providerID,
		governanceContext,
		func(operationCtx context.Context) error {
			if err := resolved.Plugin.ValidateConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			); err != nil {
				return fmt.Errorf(
					"update configured provider: validate config: %w",
					err,
				)
			}
			metadata, err := resolved.Plugin.InspectConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			)
			if err != nil {
				return fmt.Errorf(
					"update configured provider: inspect config: %w",
					err,
				)
			}
			nextConfigWithMetadata, err := nextConfig.WithMetadata(metadata)
			if err != nil {
				return fmt.Errorf(
					"update configured provider: normalize config metadata: %w",
					err,
				)
			}
			if err := r.providerConfigs.Save(
				operationCtx,
				nextConfigWithMetadata,
			); err != nil {
				return fmt.Errorf(
					"update configured provider: save config: %w",
					err,
				)
			}

			request.ProviderID = providerID
			updated, err = r.Catalog.UpdateProvider.Execute(
				operationCtx,
				request,
			)
			if err != nil {
				restoreErr := r.providerConfigs.Save(
					context.WithoutCancel(operationCtx),
					currentConfig,
				)
				return errors.Join(
					fmt.Errorf(
						"update configured provider: metadata: %w",
						err,
					),
					wrapOptionalError(
						"update configured provider: restore config",
						restoreErr,
					),
				)
			}
			return nil
		},
	)
	if activationErr != nil {
		return catalogdomain.Provider{}, activationErr
	}
	return updated, nil
}

// RebindProviderInstallation explicitly moves one external Provider to another
// verified installation of the same PluginID while preserving canonical
// Provider identity. The target Tool snapshot is proven before the persisted pin
// and execution route are retired from the current installation.
func (r *Runtime) rebindProviderInstallation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	targetInstallationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogapplication.SyncProviderToolsResult, error) {
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: provider: %w",
			err,
		)
	}
	currentConfig, err := r.providerConfigs.GetProviderConfig(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: load current config: %w",
			err,
		)
	}
	if currentConfig.InstallationID == nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: Provider %s uses a built-in plugin",
			providerID,
		)
	}
	if *currentConfig.InstallationID == targetInstallationID {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: Provider %s already uses installation %s",
			providerID,
			targetInstallationID,
		)
	}

	resolved, err := r.externalResolver.ResolveInstallation(ctx, targetInstallationID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: target: %w",
			err,
		)
	}
	if resolved.Descriptor.ID != currentConfig.PluginID {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: target plugin %s does not match Provider plugin %s",
			resolved.Descriptor.ID,
			currentConfig.PluginID,
		)
	}
	if err := resolved.Plugin.ValidateConfig(ctx, cloneRawJSON(rawConfig)); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: validate config: %w",
			err,
		)
	}
	metadata, err := resolved.Plugin.InspectConfig(ctx, cloneRawJSON(rawConfig))
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: inspect config: %w",
			err,
		)
	}
	targetConfig, err := providerplugin.NewInstalledConfiguration(
		providerID,
		currentConfig.PluginID,
		targetInstallationID,
		resolved.Descriptor.ConfigVersion,
		rawConfig,
	)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: build target config: %w",
			err,
		)
	}
	targetConfig, err = targetConfig.WithMetadata(metadata)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: normalize target metadata: %w",
			err,
		)
	}

	previousTools, err := r.Catalog.store.ListProviderTools(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: snapshot current tools: %w",
			err,
		)
	}

	candidate, err := resolved.Plugin.Open(ctx, providerplugin.OpenRequest{
		ProviderID:    providerID,
		ConfigVersion: targetConfig.ConfigVersion,
		Config:        cloneRawJSON(targetConfig.Config),
		Credentials:   r.Credentials.Resolver,
	})
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: open target: %w",
			err,
		)
	}
	if candidate == nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: target returned a nil instance",
		)
	}
	candidateClosed := false
	closeCandidate := func() error {
		if candidateClosed {
			return nil
		}
		candidateClosed = true
		return candidate.Close()
	}
	defer func() { _ = closeCandidate() }()

	syncTools := r.Catalog.SyncProviderTools(candidate)
	result, err := syncTools.Execute(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: synchronize target: %w",
			err,
		)
	}
	if err := closeCandidate(); err != nil {
		restoreErr := r.Catalog.store.ReplaceProviderTools(
			context.WithoutCancel(ctx),
			providerID,
			previousTools,
		)
		return catalogapplication.SyncProviderToolsResult{}, errors.Join(
			fmt.Errorf("rebind provider installation: close prepared target: %w", err),
			wrapOptionalError(
				"rebind provider installation: restore Tool snapshot",
				restoreErr,
			),
		)
	}

	if err := r.providerConfigs.Save(ctx, targetConfig); err != nil {
		restoreErr := r.Catalog.store.ReplaceProviderTools(
			context.WithoutCancel(ctx),
			providerID,
			previousTools,
		)
		return catalogapplication.SyncProviderToolsResult{}, errors.Join(
			fmt.Errorf("rebind provider installation: save target config: %w", err),
			wrapOptionalError(
				"rebind provider installation: restore Tool snapshot",
				restoreErr,
			),
		)
	}

	if _, err := r.providerInstances.Reload(ctx, providerID); err != nil {
		restoreConfigErr := r.providerConfigs.Save(
			context.WithoutCancel(ctx),
			currentConfig,
		)
		restoreToolsErr := r.Catalog.store.ReplaceProviderTools(
			context.WithoutCancel(ctx),
			providerID,
			previousTools,
		)
		var restoreRouteErr error
		if restoreConfigErr == nil {
			_, restoreRouteErr = r.providerInstances.Reload(
				context.WithoutCancel(ctx),
				providerID,
			)
		}
		return catalogapplication.SyncProviderToolsResult{}, errors.Join(
			fmt.Errorf("rebind provider installation: activate target: %w", err),
			wrapOptionalError(
				"rebind provider installation: restore config",
				restoreConfigErr,
			),
			wrapOptionalError(
				"rebind provider installation: restore Tool snapshot",
				restoreToolsErr,
			),
			wrapOptionalError(
				"rebind provider installation: restore execution route",
				restoreRouteErr,
			),
		)
	}
	return result, nil
}

// SyncConfiguredProvider reloads the current Provider plugin instance and
// atomically refreshes its canonical Tool snapshot.
func (r *Runtime) syncConfiguredProvider(
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

// DeleteConfiguredProvider removes generic Provider configuration and canonical
// Provider state after checking Profile/Policy references.
func (r *Runtime) DeleteConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	// Managed Agent facades cannot be torn down through Provider CRUD;
	// otherwise CloseProvider would run before the persisted guard fails.
	if r.agentStore != nil {
		managed, err := r.agentStore.IsToolFacadeProvider(ctx, providerID.String())
		if err != nil {
			return err
		}
		if managed {
			return fmt.Errorf("%w: Provider is owned by an Agent Tool Suite", ErrProviderInUse)
		}
	}
	if r.Sandboxes != nil && r.Sandboxes.store != nil {
		managed, err := r.Sandboxes.store.IsManagedSandboxSuiteProvider(ctx, providerID.String())
		if err != nil {
			return err
		}
		if managed {
			return fmt.Errorf("%w: Provider belongs to a Sandbox Tool Suite", ErrProviderInUse)
		}
	}
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
