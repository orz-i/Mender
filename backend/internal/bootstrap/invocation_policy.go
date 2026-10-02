package bootstrap

import (
	"context"
	"fmt"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

type invocationPolicyEvaluator struct {
	evaluate policyapplication.Evaluate
}

func newInvocationPolicyEvaluator(
	evaluate policyapplication.Evaluate,
) invocationapplication.PolicyEvaluator {
	return invocationPolicyEvaluator{evaluate: evaluate}
}

func (a invocationPolicyEvaluator) Evaluate(
	ctx context.Context,
	request invocationapplication.PolicyRequest,
) (policydomain.Decision, error) {
	factKnowledge, err := mapPolicyFactKnowledge(request.Facts.Knowledge())
	if err != nil {
		return policydomain.Decision{}, err
	}

	effects := make([]string, 0, len(request.Facts.Effects()))
	for _, effect := range request.Facts.Effects() {
		effects = append(effects, string(effect))
	}
	resources := make([]policyapplication.ResourceFact, 0, len(request.Facts.Resources()))
	for _, resource := range request.Facts.Resources() {
		resources = append(resources, policyapplication.ResourceFact{
			Kind:  resource.Kind(),
			Value: resource.Value(),
		})
	}

	principal := request.Context.Principal()
	scope := request.Context.Scope()
	return a.evaluate.Execute(ctx, policyapplication.EvaluateRequest{
		Action:             governancedomain.ActionToolInvoke,
		Origin:             request.Context.Origin(),
		PrincipalKind:      principal.Kind(),
		PrincipalReference: principal.Reference(),
		ScopeKind:          scope.Kind(),
		ScopeReference:     scope.Reference(),
		ProviderID:         request.Tool.ProviderID().String(),
		ToolID:             request.Tool.ID().String(),
		FactKnowledge:      factKnowledge,
		Effects:            effects,
		Resources:          resources,
	})
}

func mapPolicyFactKnowledge(
	value invocationdomain.FactKnowledge,
) (policydomain.FactKnowledge, error) {
	switch value {
	case invocationdomain.FactKnowledgeKnown:
		return policydomain.FactKnown, nil
	case invocationdomain.FactKnowledgeUnknown:
		return policydomain.FactUnknown, nil
	default:
		return "", fmt.Errorf("map policy fact knowledge: unknown value %q", value)
	}
}
