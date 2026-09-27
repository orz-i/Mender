package application

import (
	"fmt"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func applyBindingPolicyConstraint(
	decision policydomain.Decision,
	constraint invocationdomain.BindingPolicyConstraint,
) (policydomain.Decision, error) {
	switch decision.Kind() {
	case policydomain.DecisionDeny:
		return decision, nil

	case policydomain.DecisionRequireApproval:
		switch constraint {
		case invocationdomain.BindingPolicyInherit,
			invocationdomain.BindingPolicyRequireApproval:
			return decision, nil
		case invocationdomain.BindingPolicyDeny:
			return policydomain.Deny(
				"profile.binding.deny",
				"invocation denied by Profile ToolBinding",
			)
		default:
			return policydomain.Decision{}, fmt.Errorf(
				"unknown binding policy constraint %q",
				constraint,
			)
		}

	case policydomain.DecisionAllow:
		switch constraint {
		case invocationdomain.BindingPolicyInherit:
			return decision, nil
		case invocationdomain.BindingPolicyRequireApproval:
			return policydomain.RequireApproval(
				"profile.binding.require_approval",
				"Profile ToolBinding requires approval",
			)
		case invocationdomain.BindingPolicyDeny:
			return policydomain.Deny(
				"profile.binding.deny",
				"invocation denied by Profile ToolBinding",
			)
		default:
			return policydomain.Decision{}, fmt.Errorf(
				"unknown binding policy constraint %q",
				constraint,
			)
		}

	default:
		return policydomain.Decision{}, fmt.Errorf(
			"unknown global policy decision %q",
			decision.Kind(),
		)
	}
}
