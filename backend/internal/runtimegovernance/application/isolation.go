package application

import (
	"context"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

type IsolationRepository interface {
	GetIsolation(context.Context) (runtimegovernancedomain.Isolation, error)
	SetIsolation(context.Context, runtimegovernancedomain.Isolation) error
}

type GetIsolation struct{ repository IsolationRepository }

func NewGetIsolation(repository IsolationRepository) GetIsolation {
	return GetIsolation{repository: repository}
}

func (uc GetIsolation) Execute(ctx context.Context) (runtimegovernancedomain.Isolation, error) {
	return uc.repository.GetIsolation(ctx)
}

type UpdateIsolation struct{ repository IsolationRepository }

func NewUpdateIsolation(repository IsolationRepository) UpdateIsolation {
	return UpdateIsolation{repository: repository}
}

func (uc UpdateIsolation) Execute(
	ctx context.Context,
	spec runtimegovernancedomain.IsolationSpec,
) (runtimegovernancedomain.Isolation, error) {
	value, err := runtimegovernancedomain.NewIsolation(spec)
	if err != nil {
		return runtimegovernancedomain.Isolation{}, err
	}
	if err := uc.repository.SetIsolation(ctx, value); err != nil {
		return runtimegovernancedomain.Isolation{}, err
	}
	return value, nil
}
