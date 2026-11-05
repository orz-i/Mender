package application

import (
	"context"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ProviderToolSnapshots is the explicit application boundary used while
// preparing and compensating an exact plugin installation rebind. It preserves
// canonical Tool identities and delegates snapshot atomicity to Catalog.
type ProviderToolSnapshots struct{ tools ProviderToolRepository }

func NewProviderToolSnapshots(tools ProviderToolRepository) ProviderToolSnapshots {
	return ProviderToolSnapshots{tools: tools}
}

func (uc ProviderToolSnapshots) Snapshot(ctx context.Context, id domain.ProviderID) ([]domain.Tool, error) {
	return uc.tools.ListProviderTools(ctx, id)
}

func (uc ProviderToolSnapshots) Restore(ctx context.Context, id domain.ProviderID, tools []domain.Tool) error {
	return uc.tools.ReplaceProviderTools(ctx, id, tools)
}
