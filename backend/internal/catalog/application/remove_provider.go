package application

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ProviderRemover deletes one Provider and Catalog-owned Tools.
type ProviderRemover interface {
	RemoveProvider(context.Context, domain.ProviderID) error
}

// RemoveProvider removes one canonical Provider.
type RemoveProvider struct {
	providers ProviderRemover
}

func NewRemoveProvider(providers ProviderRemover) RemoveProvider {
	return RemoveProvider{providers: providers}
}

func (uc RemoveProvider) Execute(ctx context.Context, id domain.ProviderID) error {
	if err := uc.providers.RemoveProvider(ctx, id); err != nil {
		return fmt.Errorf("remove provider: %w", err)
	}
	return nil
}
