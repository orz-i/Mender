package application

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ProviderGetter loads Providers by stable Mender identity.
type ProviderGetter interface {
	GetProvider(context.Context, domain.ProviderID) (domain.Provider, error)
}

// GetProvider loads one Provider.
type GetProvider struct {
	providers ProviderGetter
}

func NewGetProvider(providers ProviderGetter) GetProvider {
	return GetProvider{providers: providers}
}

func (uc GetProvider) Execute(
	ctx context.Context,
	id domain.ProviderID,
) (domain.Provider, error) {
	provider, err := uc.providers.GetProvider(ctx, id)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("get provider: %w", err)
	}
	return provider, nil
}
