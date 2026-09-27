package domain

import (
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestScopedBindingGovernanceValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		policy      BindingPolicyConstraint
		concurrency BindingConcurrencyConstraint
	}{
		{BindingPolicyInherit, BindingConcurrencyInherit},
		{BindingPolicyRequireApproval, BindingConcurrencyInherit},
		{BindingPolicyDeny, BindingConcurrencySerial},
	}
	for _, tc := range tests {
		governance, err := NewScopedBindingGovernance(tc.policy, tc.concurrency)
		if err != nil {
			t.Fatalf("NewScopedBindingGovernance(%q,%q) error = %v", tc.policy, tc.concurrency, err)
		}
		if governance.PolicyConstraint() != tc.policy ||
			governance.ConcurrencyConstraint() != tc.concurrency {
			t.Fatalf("governance = %#v", governance)
		}
	}

	if _, err := NewScopedBindingGovernance(
		BindingPolicyConstraint("allow"),
		BindingConcurrencyInherit,
	); err == nil {
		t.Fatal("accepted binding-level allow")
	}
	if _, err := NewScopedBindingGovernance(
		BindingPolicyInherit,
		BindingConcurrencyConstraint("provider_serial"),
	); err == nil {
		t.Fatal("accepted unsupported concurrency constraint")
	}

	if err := (ScopedBindingGovernance{}).Validate(); err == nil {
		t.Fatal("zero ScopedBindingGovernance unexpectedly valid")
	}
}

func TestScopedBindingGovernanceCredentialRefsAreUniqueAndCopied(t *testing.T) {
	t.Parallel()

	first, err := credentialdomain.NewReference("github", "work")
	if err != nil {
		t.Fatal(err)
	}
	second, err := credentialdomain.NewReference("search", "prod")
	if err != nil {
		t.Fatal(err)
	}
	input := []credentialdomain.Reference{first, second}
	governance, err := NewScopedBindingGovernanceWithCredentials(
		BindingPolicyInherit,
		BindingConcurrencyInherit,
		input,
	)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = second
	got := governance.CredentialRefs()
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("CredentialRefs() = %#v", got)
	}
	got[0] = second
	if governance.CredentialRefs()[0] != first {
		t.Fatal("CredentialRefs() exposed mutable state")
	}

	if _, err := NewScopedBindingGovernanceWithCredentials(
		BindingPolicyInherit,
		BindingConcurrencyInherit,
		[]credentialdomain.Reference{first, first},
	); err == nil {
		t.Fatal("accepted duplicate CredentialRefs")
	}
}
