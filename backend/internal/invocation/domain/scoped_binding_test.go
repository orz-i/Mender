package domain

import "testing"

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
