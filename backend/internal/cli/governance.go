package cli

import (
	"context"
	"encoding/json"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func registerConfiguredProviderCLI(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := cliInvocationContext(ctx, runtime, "")
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return runtime.RegisterConfiguredProvider(
		ctx,
		request,
		pluginID,
		rawConfig,
		governanceContext,
	)
}

func registerInstalledProviderCLI(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := cliInvocationContext(ctx, runtime, "")
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return runtime.RegisterInstalledProvider(
		ctx,
		request,
		pluginID,
		installationID,
		rawConfig,
		governanceContext,
	)
}

func updateConfiguredProviderCLI(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	providerID catalogdomain.ProviderID,
	request catalogapplication.UpdateProviderRequest,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := cliInvocationContext(ctx, runtime, "")
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return runtime.UpdateConfiguredProvider(
		ctx,
		providerID,
		request,
		rawConfig,
		governanceContext,
	)
}

func syncConfiguredProviderCLI(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	providerID catalogdomain.ProviderID,
) (catalogapplication.SyncProviderToolsResult, error) {
	governanceContext, err := cliInvocationContext(ctx, runtime, "")
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return runtime.SyncConfiguredProvider(ctx, providerID, governanceContext)
}

func rebindProviderInstallationCLI(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	providerID catalogdomain.ProviderID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogapplication.SyncProviderToolsResult, error) {
	governanceContext, err := cliInvocationContext(ctx, runtime, "")
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return runtime.RebindProviderInstallation(
		ctx,
		providerID,
		installationID,
		rawConfig,
		governanceContext,
	)
}

func activateProfileProvidersCLI(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	profile profiledomain.Profile,
) error {
	scope, err := governancedomain.ProfileScope(profile.ID().String())
	if err != nil {
		return err
	}
	governanceContext, err := governancedomain.NewContext(
		governancedomain.OriginCLI,
		runtime.OwnerPrincipal(),
		scope,
	)
	if err != nil {
		return err
	}
	return runtime.ActivateProfileProviders(ctx, profile, governanceContext)
}
