package application

import (
	"context"
	"fmt"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
)

type ProviderActivationLister interface {
	ListProviderActivations(
		context.Context,
		int,
	) ([]auditdomain.ProviderActivationRecord, error)
}

type ListProviderActivations struct {
	records ProviderActivationLister
}

func NewListProviderActivations(
	records ProviderActivationLister,
) ListProviderActivations {
	return ListProviderActivations{records: records}
}

func (uc ListProviderActivations) Execute(
	ctx context.Context,
	limit int,
) ([]auditdomain.ProviderActivationRecord, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrInvalidLimit
	}
	records, err := uc.records.ListProviderActivations(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list provider activation audit records: %w", err)
	}
	return records, nil
}
