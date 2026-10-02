package application

import (
	"context"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

type BudgetRepository interface {
	GetBudget(context.Context) (runtimegovernancedomain.Budget, error)
	SetBudget(context.Context, runtimegovernancedomain.Budget) error
}

type GetBudget struct{ repository BudgetRepository }

func NewGetBudget(repository BudgetRepository) GetBudget {
	return GetBudget{repository: repository}
}

func (uc GetBudget) Execute(ctx context.Context) (runtimegovernancedomain.Budget, error) {
	return uc.repository.GetBudget(ctx)
}

type UpdateBudget struct{ repository BudgetRepository }

func NewUpdateBudget(repository BudgetRepository) UpdateBudget {
	return UpdateBudget{repository: repository}
}

func (uc UpdateBudget) Execute(
	ctx context.Context,
	spec runtimegovernancedomain.BudgetSpec,
) (runtimegovernancedomain.Budget, error) {
	value, err := runtimegovernancedomain.NewBudget(spec)
	if err != nil {
		return runtimegovernancedomain.Budget{}, err
	}
	if err := uc.repository.SetBudget(ctx, value); err != nil {
		return runtimegovernancedomain.Budget{}, err
	}
	return value, nil
}
