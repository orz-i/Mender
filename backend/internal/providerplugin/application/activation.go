package application

import (
	"context"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
)

func (s Service) admitProviderActivation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
	operation provideractivationapplication.Operation,
) error {
	if _, err := s.deps.Catalog.Get(ctx, providerID); err != nil {
		return fmt.Errorf("admit provider activation: provider: %w", err)
	}
	return s.deps.Activation.Execute(
		ctx,
		provideractivationapplication.Request{
			ProviderID: providerID,
			Context:    governanceContext,
		},
		operation,
	)
}

func (s Service) SyncConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
) (catalogapplication.SyncProviderToolsResult, error) {
	return s.SyncConfiguredProviderForOwner(ctx, providerID, governanceContext, false)
}

func (s Service) SyncConfiguredProviderForOwner(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
	resourceOwner bool,
) (catalogapplication.SyncProviderToolsResult, error) {
	if !resourceOwner {
		config, err := s.deps.Configurations.GetProviderConfig(ctx, providerID)
		if err != nil {
			return catalogapplication.SyncProviderToolsResult{}, err
		}
		if s.deps.IsManagedFacade(config.PluginID) {
			return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("%w: Provider belongs to a resource-owned Tool Suite", ErrProviderInUse)
		}
	}
	var result catalogapplication.SyncProviderToolsResult
	err := s.admitProviderActivation(
		ctx,
		providerID,
		governanceContext,
		func(operationCtx context.Context) error {
			var operationErr error
			result, operationErr = s.syncConfiguredProvider(operationCtx, providerID)
			return operationErr
		},
	)
	return result, err
}

func (s Service) ActivateConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	governanceContext governancedomain.Context,
) error {
	return s.admitProviderActivation(
		ctx,
		providerID,
		governanceContext,
		func(operationCtx context.Context) error {
			if _, err := s.deps.Instances.Ensure(operationCtx, providerID); err != nil {
				return fmt.Errorf("activate configured provider: %w", err)
			}
			return nil
		},
	)
}
