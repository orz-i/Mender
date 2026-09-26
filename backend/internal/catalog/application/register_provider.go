package application

import (
	"context"
	"fmt"
	"time"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ProviderCreator persists a newly registered Provider.
type ProviderCreator interface {
	CreateProvider(context.Context, domain.Provider) error
}

// ProviderIDGenerator allocates opaque Provider identities.
type ProviderIDGenerator interface {
	NewProviderID() (domain.ProviderID, error)
}

// Clock supplies time to application use cases.
type Clock interface {
	Now() time.Time
}

// RegisterProviderRequest contains user-owned Provider metadata.
type RegisterProviderRequest struct {
	Namespace   string
	DisplayName string
	Description string
}

// RegisterProvider registers one Provider in Catalog.
type RegisterProvider struct {
	providers ProviderCreator
	ids       ProviderIDGenerator
	clock     Clock
}

func NewRegisterProvider(
	providers ProviderCreator,
	ids ProviderIDGenerator,
	clock Clock,
) RegisterProvider {
	return RegisterProvider{
		providers: providers,
		ids:       ids,
		clock:     clock,
	}
}

func (uc RegisterProvider) Execute(
	ctx context.Context,
	request RegisterProviderRequest,
) (domain.Provider, error) {
	namespace, err := domain.NewNamespace(request.Namespace)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("register provider: %w", err)
	}

	id, err := uc.ids.NewProviderID()
	if err != nil {
		return domain.Provider{}, fmt.Errorf("register provider: allocate id: %w", err)
	}

	provider, err := domain.NewProvider(
		id,
		namespace,
		request.DisplayName,
		request.Description,
		uc.clock.Now(),
	)
	if err != nil {
		return domain.Provider{}, fmt.Errorf("register provider: %w", err)
	}

	if err := uc.providers.CreateProvider(ctx, provider); err != nil {
		return domain.Provider{}, fmt.Errorf("register provider: %w", err)
	}

	return provider, nil
}
