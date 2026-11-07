package domain

import "testing"

func TestTargetScopeIsDenyByDefaultAndExact(t *testing.T) {
	a, err := NewExecutionTarget("wrk_11111111111111111111111111111111", "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewExecutionTarget("wrk_22222222222222222222222222222222", "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	c, _ := NewExecutionTarget("wrk_11111111111111111111111111111111", "sbx_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if (TargetScope{}).Allows(a) {
		t.Fatal("zero scope widened access")
	}
	scope, err := NewTargetScope([]ExecutionTarget{a})
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Allows(a) || scope.Allows(b) || scope.Allows(c) {
		t.Fatal("target scope is not exact on both Workspace and Sandbox")
	}
	// A distinct Sandbox grant must never bypass an older per-Tool target scope.
	if scope.Allows(c) {
		t.Fatal("cross-sandbox widening")
	}
	jobs, err := NewExecutionTarget("", "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if scope.Allows(jobs) {
		t.Fatal("workspace-scoped authorization matched an unscoped Job target")
	}
}

func TestTargetScopeNormalizesAndNeverUnionsPermissions(t *testing.T) {
	a, _ := NewExecutionTarget("wrk_11111111111111111111111111111111", "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	b, _ := NewExecutionTarget("wrk_11111111111111111111111111111111", "sbx_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	narrow, _ := NewTargetScope([]ExecutionTarget{a})
	wide, _ := NewTargetScope([]ExecutionTarget{b, a})
	normalized, _ := NewTargetScope([]ExecutionTarget{a, b})
	if !wide.Equal(normalized) || narrow.Equal(wide) {
		t.Fatal("normalization/equality mismatch")
	}
	if !narrow.IsSubsetOf(wide) || wide.IsSubsetOf(narrow) {
		t.Fatal("subset relation widened permissions")
	}
	out := wide.Targets()
	out[0] = ExecutionTarget{}
	if !wide.Allows(a) || !wide.Allows(b) {
		t.Fatal("mutable copy leaked into scope")
	}
	if _, err := NewTargetScope([]ExecutionTarget{a, a}); err == nil {
		t.Fatal("accepted duplicate target")
	}
	if _, err := NewTargetScope(nil); err == nil {
		t.Fatal("accepted empty allow-all/deny-ambiguous scope")
	}
}

func TestTargetScopeRejectsUntrustedOrUnresolvedTargets(t *testing.T) {
	for _, input := range []struct{ workspace, sandbox string }{
		{"../workspace", "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"wrk_00000000000000000000000000000000", "sandbox-local"},
		{"wrk_00000000000000000000000000000000", "sbx_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		{"wrk_11111111111111111111111111111111 ", "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"", ""},
		{"*", "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"", "*"},
	} {
		if target, err := NewExecutionTarget(input.workspace, input.sandbox); err == nil {
			t.Errorf("accepted target %+v => %+v", input, target)
		}
	}
	if (ExecutionTarget{}).Validate() == nil {
		t.Fatal("zero target must be invalid")
	}
}
