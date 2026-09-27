package bootstrap

import (
	"context"
	"errors"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type invocationProfileScopeResolver struct {
	get profileapplication.GetProfileByID
}

func newInvocationProfileScopeResolver(
	get profileapplication.GetProfileByID,
) invocationapplication.ProfileScopeResolver {
	return invocationProfileScopeResolver{get: get}
}

func (a invocationProfileScopeResolver) ResolveBindingGovernance(
	ctx context.Context,
	profileReference string,
	toolID catalogdomain.ToolID,
) (invocationdomain.ScopedBindingGovernance, bool, error) {
	profileID, err := profiledomain.ParseProfileID(profileReference)
	if err != nil {
		return invocationdomain.ScopedBindingGovernance{}, false, nil
	}

	profile, err := a.get.Execute(ctx, profileID)
	if errors.Is(err, profileapplication.ErrProfileNotFound) {
		return invocationdomain.ScopedBindingGovernance{}, false, nil
	}
	if err != nil {
		return invocationdomain.ScopedBindingGovernance{}, false, fmt.Errorf(
			"resolve profile scope: %w",
			err,
		)
	}

	binding, ok := profile.Binding(toolID)
	if !ok {
		return invocationdomain.ScopedBindingGovernance{}, false, nil
	}

	policy, err := mapBindingPolicyConstraint(binding.PolicyConstraint())
	if err != nil {
		return invocationdomain.ScopedBindingGovernance{}, false, err
	}
	concurrency, err := mapBindingConcurrencyConstraint(binding.ConcurrencyConstraint())
	if err != nil {
		return invocationdomain.ScopedBindingGovernance{}, false, err
	}
	governance, err := invocationdomain.NewScopedBindingGovernanceWithCredentials(
		policy,
		concurrency,
		binding.CredentialRefs(),
	)
	if err != nil {
		return invocationdomain.ScopedBindingGovernance{}, false, fmt.Errorf(
			"resolve profile scope governance: %w",
			err,
		)
	}
	return governance, true, nil
}

func mapBindingPolicyConstraint(
	value profiledomain.PolicyConstraint,
) (invocationdomain.BindingPolicyConstraint, error) {
	switch value {
	case profiledomain.PolicyInherit:
		return invocationdomain.BindingPolicyInherit, nil
	case profiledomain.PolicyRequireApproval:
		return invocationdomain.BindingPolicyRequireApproval, nil
	case profiledomain.PolicyDeny:
		return invocationdomain.BindingPolicyDeny, nil
	default:
		return "", fmt.Errorf("resolve profile scope: unknown policy constraint %q", value)
	}
}

func mapBindingConcurrencyConstraint(
	value profiledomain.ConcurrencyConstraint,
) (invocationdomain.BindingConcurrencyConstraint, error) {
	switch value {
	case profiledomain.ConcurrencyInherit:
		return invocationdomain.BindingConcurrencyInherit, nil
	case profiledomain.ConcurrencySerial:
		return invocationdomain.BindingConcurrencySerial, nil
	default:
		return "", fmt.Errorf("resolve profile scope: unknown concurrency constraint %q", value)
	}
}
