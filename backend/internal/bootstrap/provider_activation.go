package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func (r *Runtime) admitProviderActivation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
	operation provideractivationapplication.Operation,
) error {
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		return fmt.Errorf("admit provider activation: provider: %w", err)
	}
	return r.providerActivation.Execute(
		ctx,
		provideractivationapplication.Request{
			ProviderID: providerID,
			Context:    governanceContext,
		},
		operation,
	)
}

func (r *Runtime) SyncConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
) (catalogapplication.SyncProviderToolsResult, error) {
	return r.syncConfiguredProviderForOwner(ctx, providerID, governanceContext, false)
}

// Only resource-owned Tool Suite workflows may synchronize their private
// Providers. Generic Provider sync cannot redefine a managed Tool snapshot.
func (r *Runtime) syncConfiguredProviderForOwner(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
	resourceOwner bool,
) (catalogapplication.SyncProviderToolsResult, error) {
	if !resourceOwner {
		config, err := r.providerConfigs.GetProviderConfig(ctx, providerID)
		if err != nil {
			return catalogapplication.SyncProviderToolsResult{}, err
		}
		if isManagedProviderFacade(config.PluginID) {
			return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("%w: Provider belongs to a resource-owned Tool Suite", ErrProviderInUse)
		}
	}
	var result catalogapplication.SyncProviderToolsResult
	err := r.admitProviderActivation(
		ctx,
		providerID,
		governanceContext,
		func(operationCtx context.Context) error {
			var operationErr error
			result, operationErr = r.syncConfiguredProvider(operationCtx, providerID)
			return operationErr
		},
	)
	return result, err
}

func (r *Runtime) RebindProviderInstallation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	targetInstallationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
) (catalogapplication.SyncProviderToolsResult, error) {
	var result catalogapplication.SyncProviderToolsResult
	err := r.admitProviderActivation(
		ctx,
		providerID,
		governanceContext,
		func(operationCtx context.Context) error {
			var operationErr error
			result, operationErr = r.rebindProviderInstallation(
				operationCtx,
				providerID,
				targetInstallationID,
				rawConfig,
			)
			return operationErr
		},
	)
	return result, err
}

func (r *Runtime) activateConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
) error {
	return r.admitProviderActivation(
		ctx,
		providerID,
		governanceContext,
		func(operationCtx context.Context) error {
			if _, err := r.providerInstances.Ensure(operationCtx, providerID); err != nil {
				return fmt.Errorf("activate configured provider: %w", err)
			}
			return nil
		},
	)
}
