package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	agentdelegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
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
)

// isManagedProviderFacade identifies host-owned resource Tool Suite adapters.
// Their Provider/Tool identities stay in the technical registry, never the
// user-selectable Provider creation catalogue.
func isManagedProviderFacade(id providerplugin.ID) bool {
	switch id {
	case agentdelegation.ID, workspaceplugin.ID, sshjobsplugin.ID:
		return true
	default:
		return false
	}
}

// ProviderPluginDescriptors returns independently configurable Provider drivers.
func (r *Runtime) ProviderPluginDescriptors() []providerplugin.Descriptor {
	if r == nil || r.providerPlugins == nil {
		return nil
	}
	return slices.DeleteFunc(r.providerPlugins.Descriptors(), func(d providerplugin.Descriptor) bool {
		return isManagedProviderFacade(d.ID)
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
// external installations using only persisted descriptor snapshots. Missing or invalid snapshots remain
// installation-scoped diagnostics instead of hiding unrelated healthy plugins.
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
			issues = append(issues, newProviderPluginIssue(installation.ID(), providerPluginIssueResolveDescriptor,
				fmt.Errorf("no verified extension descriptor; run mender plugin verify %s", installation.ID())))
			continue
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
	return r.providerManagement().RegisterConfiguredProvider(ctx, request, pluginID, rawConfig, governanceContext)
}

// registerConfiguredProvider admits private Tool Suite facade creation only
// when called by an Agent/Sandbox resource-owner use case, never generic
// Provider CRUD, CLI, HTTP or model-controlled arguments.
func (r *Runtime) registerConfiguredProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
	managedFacade bool,
) (catalogdomain.Provider, error) {
	if managedFacade {
		return r.providerManagement().RegisterManagedFacade(ctx, request, pluginID, rawConfig, governanceContext)
	}
	return r.providerManagement().RegisterConfiguredProvider(ctx, request, pluginID, rawConfig, governanceContext)
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
		return catalogdomain.Provider{}, fmt.Errorf("register installed provider: external plugin resolver is unavailable")
	}
	return r.providerManagement().RegisterInstalledProvider(ctx, request, pluginID, installationID, rawConfig, governanceContext)
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
	return r.providerManagement().UpdateConfiguredProvider(ctx, providerID, request, rawConfig, governanceContext)
}

// DeleteConfiguredProvider removes generic Provider configuration and canonical
// Provider state after checking Profile/Policy references.
func (r *Runtime) DeleteConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	return r.providerManagement().DeleteConfiguredProvider(ctx, providerID)
}

func (r *Runtime) ensureProviderUnused(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	return r.providerManagement().EnsureProviderUnused(ctx, providerID)
}
