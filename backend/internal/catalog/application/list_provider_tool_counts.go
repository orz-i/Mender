package application

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

type ProviderToolCountLister interface {
	ListProviderToolCounts(context.Context) (map[domain.ProviderID]int, error)
}

type ListProviderToolCounts struct {
	counts ProviderToolCountLister
}

func NewListProviderToolCounts(counts ProviderToolCountLister) ListProviderToolCounts {
	return ListProviderToolCounts{counts: counts}
}

func (uc ListProviderToolCounts) Execute(
	ctx context.Context,
) (map[domain.ProviderID]int, error) {
	counts, err := uc.counts.ListProviderToolCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list provider tool counts: %w", err)
	}
	return counts, nil
}
