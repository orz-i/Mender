package domain

import (
	"strings"
	"testing"
)

func TestToolBindingDefaultsAndNormalization(t *testing.T) {
	t.Parallel()

	toolID := mustToolID(t, "tool_00000000000000000000000000000001")
	binding, err := NewToolBinding(ToolBindingSpec{
		ToolID:              toolID,
		Direct:              true,
		Alias:               "  create_issue  ",
		TitleOverride:       "  Create production issue  ",
		DescriptionOverride: "  Create an issue in the production tracker.  ",
	})
	if err != nil {
		t.Fatalf("NewToolBinding() error = %v", err)
	}
	if binding.ToolID() != toolID || !binding.Direct() {
		t.Fatalf("identity/direct = %q/%v", binding.ToolID(), binding.Direct())
	}
	if binding.Alias() != "create_issue" {
		t.Fatalf("Alias() = %q", binding.Alias())
	}
	if binding.TitleOverride() != "Create production issue" {
		t.Fatalf("TitleOverride() = %q", binding.TitleOverride())
	}
	if binding.DescriptionOverride() != "Create an issue in the production tracker." {
		t.Fatalf("DescriptionOverride() = %q", binding.DescriptionOverride())
	}
	if binding.PolicyConstraint() != PolicyInherit {
		t.Fatalf("PolicyConstraint() = %q", binding.PolicyConstraint())
	}
	if binding.ConcurrencyConstraint() != ConcurrencyInherit {
		t.Fatalf("ConcurrencyConstraint() = %q", binding.ConcurrencyConstraint())
	}
}

func TestToolBindingValidatesAliasAndConstraints(t *testing.T) {
	t.Parallel()

	toolID := mustToolID(t, "tool_00000000000000000000000000000001")

	for _, alias := range []string{
		"_bad",
		"bad_",
		"bad alias",
		"bad/slash",
		strings.Repeat("a", maxBindingAliasLength+1),
	} {
		_, err := NewToolBinding(ToolBindingSpec{ToolID: toolID, Alias: alias})
		if err == nil {
			t.Fatalf("NewToolBinding(alias=%q) expected error", alias)
		}
	}

	for _, alias := range []string{"a", "create_issue", "issue.create-v2", "A1"} {
		if _, err := NewToolBinding(ToolBindingSpec{ToolID: toolID, Alias: alias}); err != nil {
			t.Fatalf("NewToolBinding(alias=%q) error = %v", alias, err)
		}
	}

	if _, err := NewToolBinding(ToolBindingSpec{
		ToolID:           toolID,
		PolicyConstraint: PolicyConstraint("allow"),
	}); err == nil {
		t.Fatal("NewToolBinding() accepted binding-level allow")
	}
	if _, err := NewToolBinding(ToolBindingSpec{
		ToolID:                toolID,
		ConcurrencyConstraint: ConcurrencyConstraint("provider_serial"),
	}); err == nil {
		t.Fatal("NewToolBinding() accepted unsupported concurrency constraint")
	}

	for _, policy := range []PolicyConstraint{
		PolicyInherit,
		PolicyRequireApproval,
		PolicyDeny,
	} {
		for _, concurrency := range []ConcurrencyConstraint{
			ConcurrencyInherit,
			ConcurrencySerial,
		} {
			if _, err := NewToolBinding(ToolBindingSpec{
				ToolID:                toolID,
				PolicyConstraint:      policy,
				ConcurrencyConstraint: concurrency,
			}); err != nil {
				t.Fatalf(
					"NewToolBinding(policy=%q concurrency=%q) error = %v",
					policy,
					concurrency,
					err,
				)
			}
		}
	}
}

func TestToolBindingValidatesPresentationBounds(t *testing.T) {
	t.Parallel()

	toolID := mustToolID(t, "tool_00000000000000000000000000000001")
	if _, err := NewToolBinding(ToolBindingSpec{
		ToolID:        toolID,
		TitleOverride: strings.Repeat("t", maxBindingTitleLength+1),
	}); err == nil {
		t.Fatal("NewToolBinding() accepted oversized title override")
	}
	if _, err := NewToolBinding(ToolBindingSpec{
		ToolID:              toolID,
		DescriptionOverride: strings.Repeat("d", maxBindingDescriptionLength+1),
	}); err == nil {
		t.Fatal("NewToolBinding() accepted oversized description override")
	}
}

func TestProfileWithBindingsPreservesGovernanceAndNormalizesExposure(t *testing.T) {
	t.Parallel()

	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")
	toolID := mustToolID(t, "tool_00000000000000000000000000000001")
	binding, err := NewToolBinding(ToolBindingSpec{
		ToolID:                toolID,
		Direct:                false,
		Alias:                 "prod_issue",
		TitleOverride:         "Production issue",
		DescriptionOverride:   "Create production issues.",
		PolicyConstraint:      PolicyRequireApproval,
		ConcurrencyConstraint: ConcurrencySerial,
	})
	if err != nil {
		t.Fatal(err)
	}

	profile, err := NewProfileWithBindings(
		id,
		slug,
		"Default",
		ExposureDirect,
		[]ToolBinding{binding},
	)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := profile.Binding(toolID)
	if !ok {
		t.Fatal("Binding() did not find Tool")
	}
	if !got.Direct() {
		t.Fatal("direct Profile did not normalize binding to Direct")
	}
	if got.Alias() != "prod_issue" ||
		got.TitleOverride() != "Production issue" ||
		got.DescriptionOverride() != "Create production issues." ||
		got.PolicyConstraint() != PolicyRequireApproval ||
		got.ConcurrencyConstraint() != ConcurrencySerial {
		t.Fatalf("binding governance was not preserved: %#v", got)
	}
}

func TestProfileWithBindingsRejectsDuplicateToolIDs(t *testing.T) {
	t.Parallel()

	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")
	toolID := mustToolID(t, "tool_00000000000000000000000000000001")
	first, _ := DefaultToolBinding(toolID, true)
	second, _ := NewToolBinding(ToolBindingSpec{
		ToolID:           toolID,
		Alias:            "alternate",
		PolicyConstraint: PolicyDeny,
	})

	if _, err := NewProfileWithBindings(
		id,
		slug,
		"Default",
		ExposureHybrid,
		[]ToolBinding{first, second},
	); err == nil {
		t.Fatal("NewProfileWithBindings() accepted duplicate ToolID")
	}
}
