package bootstrap

import (
	"context"
	"fmt"

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
	origin, err := mapPolicyOrigin(request.Context.Origin())
	if err != nil {
		return policydomain.Decision{}, err
	}
	scopeKind, err := mapPolicyScopeKind(request.Context.Scope().Kind())
	if err != nil {
		return policydomain.Decision{}, err
	}
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

	return a.evaluate.Execute(ctx, policyapplication.EvaluateRequest{
		Origin:         origin,
		ScopeKind:      scopeKind,
		ScopeReference: request.Context.Scope().Reference(),
		ProviderID:     request.Tool.ProviderID().String(),
		ToolID:         request.Tool.ID().String(),
		FactKnowledge:  factKnowledge,
		Effects:        effects,
		Resources:      resources,
	})
}

func mapPolicyOrigin(value invocationdomain.Origin) (policydomain.Origin, error) {
	switch value {
	case invocationdomain.OriginCLI:
		return policydomain.OriginCLI, nil
	case invocationdomain.OriginMCP:
		return policydomain.OriginMCP, nil
	case invocationdomain.OriginManagementHTTP:
		return policydomain.OriginManagementHTTP, nil
	case invocationdomain.OriginDesktop:
		return policydomain.OriginDesktop, nil
	case invocationdomain.OriginInternal:
		return policydomain.OriginInternal, nil
	default:
		return "", fmt.Errorf("map policy origin: unknown value %q", value)
	}
}

func mapPolicyScopeKind(
	value invocationdomain.ScopeKind,
) (policydomain.ScopeKind, error) {
	switch value {
	case invocationdomain.ScopeCatalog:
		return policydomain.ScopeCatalog, nil
	case invocationdomain.ScopeProfile:
		return policydomain.ScopeProfile, nil
	default:
		return "", fmt.Errorf("map policy scope: unknown value %q", value)
	}
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
