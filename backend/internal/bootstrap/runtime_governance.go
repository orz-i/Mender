package bootstrap

import (
	"context"
	"fmt"

	runtimegovernanceapplication "github.com/orz-i/Mender/backend/internal/runtimegovernance/application"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	runtimebudget "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/budget"
	runtimeprocessisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
	runtimegovernancesqlite "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/sqlite"
)

// RuntimeGovernance composes persistent Runtime capacity governance.
type RuntimeGovernance struct {
	GetBudget    runtimegovernanceapplication.GetBudget
	UpdateBudget runtimegovernanceapplication.UpdateBudget
	GetIsolation runtimegovernanceapplication.GetIsolation

	updateIsolation  runtimegovernanceapplication.UpdateIsolation
	controller       *runtimebudget.Controller
	processIsolation *runtimeprocessisolation.Launcher
	store            *runtimegovernancesqlite.Store
}

func NewRuntimeGovernance(
	ctx context.Context,
	databasePath string,
) (*RuntimeGovernance, error) {
	processIsolation := runtimeprocessisolation.New()
	defaultIsolation := runtimegovernancedomain.DefaultIsolation(
		processIsolation.Info().RestrictedHostAvailable,
	)
	store, err := runtimegovernancesqlite.Open(ctx, databasePath, defaultIsolation)
	if err != nil {
		return nil, fmt.Errorf("compose runtime governance: %w", err)
	}
	return &RuntimeGovernance{
		GetBudget:        runtimegovernanceapplication.NewGetBudget(store),
		UpdateBudget:     runtimegovernanceapplication.NewUpdateBudget(store),
		GetIsolation:     runtimegovernanceapplication.NewGetIsolation(store),
		updateIsolation:  runtimegovernanceapplication.NewUpdateIsolation(store),
		controller:       runtimebudget.NewController(store),
		processIsolation: processIsolation,
		store:            store,
	}, nil
}

func (g *RuntimeGovernance) Close() error {
	if g == nil || g.store == nil {
		return nil
	}
	return g.store.Close()
}

func (g *RuntimeGovernance) UpdateIsolation(
	ctx context.Context,
	spec runtimegovernancedomain.IsolationSpec,
) (runtimegovernancedomain.Isolation, error) {
	if g == nil || g.processIsolation == nil {
		return runtimegovernancedomain.Isolation{}, fmt.Errorf("runtime isolation backend is unavailable")
	}
	if (spec.WorkspaceShell == runtimegovernancedomain.IsolationRestrictedHost ||
		spec.ExternalProvider == runtimegovernancedomain.IsolationRestrictedHost) &&
		!g.processIsolation.Info().RestrictedHostAvailable {
		return runtimegovernancedomain.Isolation{}, fmt.Errorf("restricted_host process isolation is unavailable on this Runtime")
	}
	return g.updateIsolation.Execute(ctx, spec)
}

func (g *RuntimeGovernance) IsolationBackend() runtimeprocessisolation.BackendInfo {
	if g == nil || g.processIsolation == nil {
		return runtimeprocessisolation.BackendInfo{Name: "none"}
	}
	return g.processIsolation.Info()
}

func (g *RuntimeGovernance) BudgetUsage() runtimebudget.Usage {
	if g == nil || g.controller == nil {
		return runtimebudget.Usage{}
	}
	return g.controller.Usage()
}
