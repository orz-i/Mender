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
	// Managed Agent facades cannot be torn down through Provider CRUD;
	// otherwise CloseProvider would run before the persisted guard fails.
	if s.deps.AgentOwnership != nil {
		managed, err := s.deps.AgentOwnership.Execute(ctx, providerID)
		if err != nil {
			return err
		}
		if managed {
			return fmt.Errorf("%w: Provider is owned by an Agent Tool Suite", ErrProviderInUse)
		}
	}
	if s.deps.SandboxOwnership != nil {
		managed, err := s.deps.SandboxOwnership.Execute(ctx, providerID)
		if err != nil {
			return err
		}
		if managed {
			return fmt.Errorf("%w: Provider belongs to a Sandbox Tool Suite", ErrProviderInUse)
		}
	}
	if err := s.EnsureProviderUnused(ctx, providerID); err != nil {
		return err
	}

	if err := s.deps.Instances.CloseProvider(providerID); err != nil {
		return fmt.Errorf("delete configured provider: close active instance: %w", err)
	}
	return s.deps.Retirement.Delete(ctx, providerID)
}
