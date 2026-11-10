package application

import (
	"context"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func (s Service) syncConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (catalogapplication.SyncProviderToolsResult, error) {
	if _, err := s.deps.Catalog.Get(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"sync configured provider: provider: %w",
			err,
		)
	}

	instance, err := s.deps.Instances.Reload(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"sync configured provider: activate: %w",
			err,
		)
	}
	result, err := s.deps.Catalog.Sync(ctx, providerID, instance)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"sync configured provider: synchronize: %w",
			err,
		)
	}
	return result, nil
}

func (s Service) DeleteConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	if err := s.EnsureProviderUnused(ctx, providerID); err != nil {
		return err
	}

	if err := s.deps.Instances.CloseProvider(providerID); err != nil {
		return fmt.Errorf("delete configured provider: close active instance: %w", err)
	}
	return s.deps.Retirement.Delete(ctx, providerID)
}
