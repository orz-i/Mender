package bootstrap

import (
	"context"
	"errors"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policyidentity "github.com/orz-i/Mender/backend/internal/policy/infrastructure/identity"
	policysqlite "github.com/orz-i/Mender/backend/internal/policy/infrastructure/sqlite"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// Policies composes persistent global Policy management and evaluation.
type Policies struct {
	Get            policyapplication.GetPolicy
	SetDefault     policyapplication.SetDefaultDecision
	CreateRule     policyapplication.CreateRule
	UpdateRule     policyapplication.UpdateRule
	DeleteRule     policyapplication.DeleteRule
	SetRuleEnabled policyapplication.SetRuleEnabled
	Evaluate       policyapplication.Evaluate

	store *policysqlite.Store
}

func NewPolicies(
	ctx context.Context,
	databasePath string,
	catalog *Catalog,
	profiles *Profiles,
) (*Policies, error) {
	store, err := policysqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose policy: %w", err)
	}
	ids := policyidentity.NewGenerator()
	clock := systemClock{}
	references := policyReferenceLookup{
		catalog:  catalog,
		profiles: profiles,
	}
	return &Policies{
		Get:            policyapplication.NewGetPolicy(store),
		SetDefault:     policyapplication.NewSetDefaultDecision(store, clock),
		CreateRule:     policyapplication.NewCreateRule(store, ids, clock, references),
		UpdateRule:     policyapplication.NewUpdateRule(store, clock, references),
		DeleteRule:     policyapplication.NewDeleteRule(store),
		SetRuleEnabled: policyapplication.NewSetRuleEnabled(store, clock),
		Evaluate:       policyapplication.NewEvaluate(store),
		store:          store,
	}, nil
}

func (p *Policies) Close() error {
	return p.store.Close()
}

type policyReferenceLookup struct {
	catalog  *Catalog
	profiles *Profiles
}

func (a policyReferenceLookup) ProfileExists(
	ctx context.Context,
	reference string,
) (bool, error) {
	id, err := profiledomain.ParseProfileID(reference)
	if err != nil {
		return false, nil
	}
	_, err = a.profiles.GetByID.Execute(ctx, id)
	if errors.Is(err, profileapplication.ErrProfileNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a policyReferenceLookup) ProviderExists(
	ctx context.Context,
	reference string,
) (bool, error) {
	id, err := catalogdomain.ParseProviderID(reference)
	if err != nil {
		return false, nil
	}
	_, err = a.catalog.GetProvider.Execute(ctx, id)
	if errors.Is(err, catalogapplication.ErrProviderNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a policyReferenceLookup) ToolExists(
	ctx context.Context,
	reference string,
) (bool, error) {
	id, err := catalogdomain.ParseToolID(reference)
	if err != nil {
		return false, nil
	}
	_, err = a.catalog.DescribeTool.Execute(ctx, id)
	if errors.Is(err, catalogapplication.ErrToolNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

var _ policyapplication.ReferenceLookup = policyReferenceLookup{}
