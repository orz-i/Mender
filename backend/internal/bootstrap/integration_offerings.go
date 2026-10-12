package bootstrap

import (
	"context"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/integration/domain"
	"github.com/orz-i/Mender/backend/internal/integration/infrastructure/curated"
)

// ListIntegrationOfferings is a process-free snapshot, not proof of
// reachability or user authorization.
func (r *Runtime) ListIntegrationOfferings(ctx context.Context) ([]domain.Offering, error) {
	if r == nil {
		return nil, fmt.Errorf("integration offerings: runtime unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := curated.List()
	for _, item := range result {
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("invalid reviewed integration %s: %w", item.OfferingID, err)
		}
	}
	return result, nil
}
