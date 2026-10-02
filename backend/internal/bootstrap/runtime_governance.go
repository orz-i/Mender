package bootstrap

import (
	"context"
	"fmt"

	runtimegovernanceapplication "github.com/orz-i/Mender/backend/internal/runtimegovernance/application"
	runtimebudget "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/budget"
	runtimegovernancesqlite "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/sqlite"
)

// RuntimeGovernance composes persistent Runtime capacity governance.
type RuntimeGovernance struct {
	GetBudget    runtimegovernanceapplication.GetBudget
	UpdateBudget runtimegovernanceapplication.UpdateBudget

	controller *runtimebudget.Controller
	store      *runtimegovernancesqlite.Store
}

func NewRuntimeGovernance(
	ctx context.Context,
	databasePath string,
) (*RuntimeGovernance, error) {
	store, err := runtimegovernancesqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose runtime governance: %w", err)
	}
	return &RuntimeGovernance{
		GetBudget:    runtimegovernanceapplication.NewGetBudget(store),
		UpdateBudget: runtimegovernanceapplication.NewUpdateBudget(store),
		controller:   runtimebudget.NewController(store),
		store:        store,
	}, nil
}

func (g *RuntimeGovernance) Close() error {
	if g == nil || g.store == nil {
		return nil
	}
	return g.store.Close()
}

func (g *RuntimeGovernance) BudgetUsage() runtimebudget.Usage {
	if g == nil || g.controller == nil {
		return runtimebudget.Usage{}
	}
	return g.controller.Usage()
}
