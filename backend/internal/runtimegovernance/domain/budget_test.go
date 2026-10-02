package domain

import "testing"

func TestDefaultBudgetIsValid(t *testing.T) {
	t.Parallel()
	budget := DefaultBudget()
	if err := budget.Validate(); err != nil {
		t.Fatal(err)
	}
	if budget.MaxInvocations() != 32 ||
		budget.MaxPrincipalInvocations() != 8 ||
		budget.MaxProviderInvocations() != 8 ||
		budget.MaxToolInvocations() != 4 ||
		budget.MaxProviderActivations() != 4 ||
		budget.MaxExecutionTimeoutMS() != 120000 ||
		budget.MaxArgumentBytes() != 1<<20 ||
		budget.MaxResultBytes() != 1<<20 {
		t.Fatalf("default budget = %#v", budget.Spec())
	}
}

func TestBudgetRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	spec := DefaultBudget().Spec()
	spec.MaxInvocations = 0
	if _, err := NewBudget(spec); err == nil {
		t.Fatal("NewBudget accepted zero concurrency")
	}
	spec = DefaultBudget().Spec()
	spec.MaxExecutionTimeoutMS = 0
	if _, err := NewBudget(spec); err == nil {
		t.Fatal("NewBudget accepted zero timeout")
	}
	spec = DefaultBudget().Spec()
	spec.MaxArgumentBytes = 0
	if _, err := NewBudget(spec); err == nil {
		t.Fatal("NewBudget accepted zero argument limit")
	}
}
