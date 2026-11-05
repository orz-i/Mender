package domain

import (
	"slices"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestProfileSelectionPreservesExactMembers(t *testing.T) {
	t.Parallel()
	first := mustToolID(t, "tool_00000000000000000000000000000001")
	second := mustToolID(t, "tool_00000000000000000000000000000002")
	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")
	p, err := NewProfileWithBindings(id, slug, "Default", []ToolBinding{mustBinding(t, first), mustBinding(t, second)})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.ToolIDs(), []catalogdomain.ToolID{first, second}) || !p.Contains(first) || !p.Contains(second) {
		t.Fatalf("selection changed: %+v", p.Bindings())
	}
}

func TestProfileRejectsDuplicateToolMembership(t *testing.T) {
	t.Parallel()

	toolID := mustToolID(t, "tool_00000000000000000000000000000001")
	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")

	_, err := NewProfileWithBindings(
		id,
		slug,
		"Default",

		[]ToolBinding{
			mustBinding(t, toolID),
			mustBinding(t, toolID),
		},
	)
	if err == nil {
		t.Fatal("NewProfileWithBindings() expected duplicate ToolID error")
	}
}

func TestProfileIDAndSlugValidation(t *testing.T) {
	t.Parallel()

	if _, err := ParseProfileID("prof_0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("ParseProfileID() error = %v", err)
	}
	if _, err := ParseProfileID("profile_0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("ParseProfileID() expected prefix error")
	}

	for _, value := range []string{"default", "team-a", "p1"} {
		if _, err := NewSlug(value); err != nil {
			t.Errorf("NewSlug(%q) error = %v", value, err)
		}
	}
	for _, value := range []string{"", "Default", "-bad", "bad-", "bad_slug"} {
		if _, err := NewSlug(value); err == nil {
			t.Errorf("NewSlug(%q) expected error", value)
		}
	}
}

func mustToolID(t *testing.T, raw string) catalogdomain.ToolID {
	t.Helper()
	id, err := catalogdomain.ParseToolID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustBinding(t *testing.T, toolID catalogdomain.ToolID) ToolBinding {
	t.Helper()
	binding, err := NewToolBinding(ToolBindingSpec{ToolID: toolID})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}
