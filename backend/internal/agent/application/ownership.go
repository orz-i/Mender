package application

import (
	"context"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

type FacadeOwnershipLookup interface {
	IsToolFacadeProvider(context.Context, string) (bool, error)
}

// FacadeOwnership exposes only the Agent-owned Provider relationship, without
// widening the Agent Repository contract or exposing its persistence adapter.
type FacadeOwnership struct{ lookup FacadeOwnershipLookup }

func NewFacadeOwnership(lookup FacadeOwnershipLookup) FacadeOwnership {
	return FacadeOwnership{lookup: lookup}
}

func (q FacadeOwnership) Execute(ctx context.Context, providerID catalogdomain.ProviderID) (bool, error) {
	return q.lookup.IsToolFacadeProvider(ctx, providerID.String())
}
