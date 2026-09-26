package application

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ProviderNamespaceFinder loads one Provider by stable Namespace.
type ProviderNamespaceFinder interface {
	FindProviderByNamespace(context.Context, domain.Namespace) (domain.Provider, error)
}

// FindProviderByNamespace resolves one Provider by Namespace.
type FindProviderByNamespace struct {
	providers ProviderNamespaceFinder
}

func NewFindProviderByNamespace(providers ProviderNamespaceFinder) FindProviderByNamespace {
	return FindProviderByNamespace{providers: providers}
}

func (uc FindProviderByNamespace) Execute(
	ctx context.Context,
	namespace domain.Namespace,
) (domain.Provider, error) {
	provider, err := uc.providers.FindProviderByNamespace(ctx, namespace)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("find provider by namespace: %w", err)
	}
	return provider, nil
}
