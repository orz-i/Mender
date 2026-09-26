package application

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ProviderLister loads all registered Providers.
type ProviderLister interface {
	ListProviders(context.Context) ([]domain.Provider, error)
}

// ListProviders returns Providers ordered by repository-defined stable order.
type ListProviders struct {
	providers ProviderLister
}

func NewListProviders(providers ProviderLister) ListProviders {
	return ListProviders{providers: providers}
}

func (uc ListProviders) Execute(ctx context.Context) ([]domain.Provider, error) {
	providers, err := uc.providers.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	return providers, nil
}
