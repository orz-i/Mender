package domain

import "testing"

func TestProfileBindingMutationsPreserveExistingGovernance(t *testing.T) {
	t.Parallel()

	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")
	firstID := mustToolID(t, "tool_00000000000000000000000000000001")
	secondID := mustToolID(t, "tool_00000000000000000000000000000002")

	first, err := NewToolBinding(ToolBindingSpec{
		ToolID:                firstID,
		Alias:                 "first",
		PolicyConstraint:      PolicyDeny,
		ConcurrencyConstraint: ConcurrencySerial,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := NewProfileWithBindings(
		id,
		slug,
		"Default",
		ExposureHybrid,
		[]ToolBinding{first},
	)
	if err != nil {
		t.Fatal(err)
	}

	second, err := NewToolBinding(ToolBindingSpec{
		ToolID:        secondID,
		Direct:        true,
		TitleOverride: "Second Tool",
	})
	if err != nil {
		t.Fatal(err)
	}
	added, err := profile.AddBinding(second)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Contains(secondID) {
		t.Fatal("AddBinding() mutated original Profile")
	}
	preserved, ok := added.Binding(firstID)
	if !ok ||
		preserved.Alias() != "first" ||
		preserved.PolicyConstraint() != PolicyDeny ||
		preserved.ConcurrencyConstraint() != ConcurrencySerial {
		t.Fatalf("existing binding governance changed: %#v", preserved)
	}

	replacement, err := NewToolBinding(ToolBindingSpec{
		ToolID:           secondID,
		Alias:            "second",
		PolicyConstraint: PolicyRequireApproval,
	})
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := added.ReplaceBindings([]ToolBinding{replacement})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Contains(firstID) {
		t.Fatal("ReplaceBindings() retained omitted binding")
	}
	got, ok := replaced.Binding(secondID)
	if !ok || got.Alias() != "second" || got.PolicyConstraint() != PolicyRequireApproval {
		t.Fatalf("replacement binding = %#v", got)
	}

	removed, err := added.RemoveTool(firstID)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Contains(firstID) || !removed.Contains(secondID) {
		t.Fatalf("RemoveTool() result bindings = %#v", removed.Bindings())
	}
	secondAfterRemove, _ := removed.Binding(secondID)
	if secondAfterRemove.TitleOverride() != "Second Tool" {
		t.Fatalf("RemoveTool() changed remaining binding = %#v", secondAfterRemove)
	}
}
