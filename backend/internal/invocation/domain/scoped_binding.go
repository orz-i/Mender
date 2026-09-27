package domain

import (
	"fmt"
	"slices"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

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
	credentialRefs        []credentialdomain.Reference
}

// NewScopedBindingGovernance validates one Profile ToolBinding governance projection.
func NewScopedBindingGovernance(
	policyConstraint BindingPolicyConstraint,
	concurrencyConstraint BindingConcurrencyConstraint,
) (ScopedBindingGovernance, error) {
	return NewScopedBindingGovernanceWithCredentials(
		policyConstraint,
		concurrencyConstraint,
		nil,
	)
}

func NewScopedBindingGovernanceWithCredentials(
	policyConstraint BindingPolicyConstraint,
	concurrencyConstraint BindingConcurrencyConstraint,
	credentialRefs []credentialdomain.Reference,
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
	refs := make([]credentialdomain.Reference, 0, len(credentialRefs))
	seen := make(map[credentialdomain.Reference]struct{}, len(credentialRefs))
	for _, ref := range credentialRefs {
		parsed, err := credentialdomain.ParseReference(ref.String())
		if err != nil {
			return ScopedBindingGovernance{}, fmt.Errorf(
				"scoped binding governance: credential ref: %w",
				err,
			)
		}
		if _, exists := seen[parsed]; exists {
			return ScopedBindingGovernance{}, fmt.Errorf(
				"scoped binding governance: duplicate credential ref %s",
				parsed,
			)
		}
		seen[parsed] = struct{}{}
		refs = append(refs, parsed)
	}

	return ScopedBindingGovernance{
		policyConstraint:      policyConstraint,
		concurrencyConstraint: concurrencyConstraint,
		credentialRefs:        refs,
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
	_, err := NewScopedBindingGovernanceWithCredentials(
		g.policyConstraint,
		g.concurrencyConstraint,
		g.credentialRefs,
	)
	return err
}

func (g ScopedBindingGovernance) PolicyConstraint() BindingPolicyConstraint {
	return g.policyConstraint
}

func (g ScopedBindingGovernance) ConcurrencyConstraint() BindingConcurrencyConstraint {
	return g.concurrencyConstraint
}

func (g ScopedBindingGovernance) CredentialRefs() []credentialdomain.Reference {
	return slices.Clone(g.credentialRefs)
}
