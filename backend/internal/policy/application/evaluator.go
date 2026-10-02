package application

import (
	"context"
	"fmt"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// EvaluateRequest is the bounded application boundary consumed by Runtime adapters.
// It intentionally contains no raw Tool arguments, Credential references, or secret material.
type EvaluateRequest struct {
	Action             governancedomain.Action
	Origin             governancedomain.Origin
	PrincipalKind      governancedomain.PrincipalKind
	PrincipalReference string
	ScopeKind          governancedomain.ScopeKind
	ScopeReference     string
	ProviderID         string
	ToolID             string
	FactKnowledge      policydomain.FactKnowledge
	Effects            []string
	Resources          []ResourceFact
}

// ResourceFact is the Policy-facing bounded resource projection.
type ResourceFact struct {
	Kind  string
	Value string
}

type Evaluate struct {
	repository Repository
}

func NewEvaluate(repository Repository) Evaluate {
	return Evaluate{repository: repository}
}

func (uc Evaluate) Execute(
	ctx context.Context,
	request EvaluateRequest,
) (policydomain.Decision, error) {
	resources := make([]policydomain.ResourceFact, 0, len(request.Resources))
	for _, resource := range request.Resources {
		resources = append(resources, policydomain.ResourceFact{
			Kind:  resource.Kind,
			Value: resource.Value,
		})
	}

	policy, err := uc.repository.GetPolicySet(ctx)
	if err != nil {
		return policydomain.Decision{}, fmt.Errorf("evaluate policy: %w", err)
	}
	decision, err := policy.Evaluate(policydomain.EvaluationInput{
		Action:             request.Action,
		Origin:             request.Origin,
		PrincipalKind:      request.PrincipalKind,
		PrincipalReference: request.PrincipalReference,
		ScopeKind:          request.ScopeKind,
		ScopeReference:     request.ScopeReference,
		ProviderID:         request.ProviderID,
		ToolID:             request.ToolID,
		FactKnowledge:      request.FactKnowledge,
		Effects:            append([]string(nil), request.Effects...),
		Resources:          resources,
	})
	if err != nil {
		return policydomain.Decision{}, fmt.Errorf("evaluate policy: %w", err)
	}
	return decision, nil
}
