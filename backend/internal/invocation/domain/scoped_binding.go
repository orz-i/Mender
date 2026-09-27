package domain

import "fmt"

// BindingPolicyConstraint is the Invocation-owned normalized Profile policy constraint.
type BindingPolicyConstraint string

const (
	BindingPolicyInherit         BindingPolicyConstraint = "inherit"
	BindingPolicyRequireApproval BindingPolicyConstraint = "require_approval"
	BindingPolicyDeny            BindingPolicyConstraint = "deny"
)

// BindingConcurrencyConstraint is the Invocation-owned normalized Profile concurrency constraint.
type BindingConcurrencyConstraint string

const (
	BindingConcurrencyInherit BindingConcurrencyConstraint = "inherit"
	BindingConcurrencySerial  BindingConcurrencyConstraint = "serial"
)

// ScopedBindingGovernance contains execution-relevant Profile ToolBinding state.
type ScopedBindingGovernance struct {
	policyConstraint      BindingPolicyConstraint
	concurrencyConstraint BindingConcurrencyConstraint
}

// NewScopedBindingGovernance validates one Profile ToolBinding governance projection.
func NewScopedBindingGovernance(
	policyConstraint BindingPolicyConstraint,
	concurrencyConstraint BindingConcurrencyConstraint,
) (ScopedBindingGovernance, error) {
	switch policyConstraint {
	case BindingPolicyInherit, BindingPolicyRequireApproval, BindingPolicyDeny:
	default:
		return ScopedBindingGovernance{}, fmt.Errorf(
			"scoped binding governance: unknown policy constraint %q",
			policyConstraint,
		)
	}
	switch concurrencyConstraint {
	case BindingConcurrencyInherit, BindingConcurrencySerial:
	default:
		return ScopedBindingGovernance{}, fmt.Errorf(
			"scoped binding governance: unknown concurrency constraint %q",
			concurrencyConstraint,
		)
	}
	return ScopedBindingGovernance{
		policyConstraint:      policyConstraint,
		concurrencyConstraint: concurrencyConstraint,
	}, nil
}

// InheritedScopedBindingGovernance returns the no-additional-constraint projection.
func InheritedScopedBindingGovernance() ScopedBindingGovernance {
	governance, err := NewScopedBindingGovernance(
		BindingPolicyInherit,
		BindingConcurrencyInherit,
	)
	if err != nil {
		panic("invocation domain: construct inherited scoped binding governance: " + err.Error())
	}
	return governance
}

func (g ScopedBindingGovernance) Validate() error {
	_, err := NewScopedBindingGovernance(
		g.policyConstraint,
		g.concurrencyConstraint,
	)
	return err
}

func (g ScopedBindingGovernance) PolicyConstraint() BindingPolicyConstraint {
	return g.policyConstraint
}

func (g ScopedBindingGovernance) ConcurrencyConstraint() BindingConcurrencyConstraint {
	return g.concurrencyConstraint
}
