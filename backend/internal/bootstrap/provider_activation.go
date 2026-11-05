package bootstrap

import (
	"context"
	"encoding/json"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func (r *Runtime) SyncConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
) (catalogapplication.SyncProviderToolsResult, error) {
	return r.providerManagement().SyncConfiguredProvider(ctx, providerID, governanceContext)
}

// Only resource-owned Tool Suite workflows may synchronize their private
// Providers. Generic Provider sync cannot redefine a managed Tool snapshot.
func (r *Runtime) syncConfiguredProviderForOwner(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
	resourceOwner bool,
) (catalogapplication.SyncProviderToolsResult, error) {
	return r.providerManagement().SyncConfiguredProviderForOwner(ctx, providerID, governanceContext, resourceOwner)
}

func (r *Runtime) RebindProviderInstallation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	targetInstallationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
) (catalogapplication.SyncProviderToolsResult, error) {
	return r.providerManagement().RebindProviderInstallation(ctx, providerID, targetInstallationID, rawConfig, governanceContext)
}

func (r *Runtime) activateConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
) error {
	return r.providerManagement().ActivateConfiguredProvider(ctx, providerID, governanceContext)
}
