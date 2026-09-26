package application

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

type ProviderUpdater interface {
	UpdateProvider(context.Context, domain.Provider) error
}

type UpdateProviderRequest struct {
	ProviderID  domain.ProviderID
	DisplayName string
	Description string
}

type UpdateProvider struct {
	getter  ProviderGetter
	updater ProviderUpdater
	clock   Clock
}

func NewUpdateProvider(
	getter ProviderGetter,
	updater ProviderUpdater,
	clock Clock,
) UpdateProvider {
	return UpdateProvider{
		getter:  getter,
		updater: updater,
		clock:   clock,
	}
}

func (uc UpdateProvider) Execute(
	ctx context.Context,
	request UpdateProviderRequest,
) (domain.Provider, error) {
	current, err := uc.getter.GetProvider(ctx, request.ProviderID)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("update provider: %w", err)
	}

	updated, err := current.UpdateMetadata(
		request.DisplayName,
		request.Description,
		uc.clock.Now(),
	)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("update provider: %w", err)
	}

	if err := uc.updater.UpdateProvider(ctx, updated); err != nil {
		return domain.Provider{}, fmt.Errorf("update provider: %w", err)
	}
	return updated, nil
}
