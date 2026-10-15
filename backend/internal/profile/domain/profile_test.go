package domain

import (
	"slices"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestProfileExposureModesNormalizeDirectSelection(t *testing.T) {
	t.Parallel()

	first := mustToolID(t, "tool_00000000000000000000000000000001")
	second := mustToolID(t, "tool_00000000000000000000000000000002")
	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")

	tests := []struct {
		name       string
		mode       ExposureMode
		selections []ToolBinding
		wantDirect []catalogdomain.ToolID
	}{
		{
			name: "direct",
			mode: ExposureDirect,
			selections: []ToolBinding{
				mustBinding(t, first, false),
				mustBinding(t, second, false),
			},
			wantDirect: []catalogdomain.ToolID{first, second},
		},
		{
			name: "dynamic",
			mode: ExposureDynamic,
			selections: []ToolBinding{
				mustBinding(t, first, true),
				mustBinding(t, second, true),
			},
			wantDirect: nil,
		},
		{
			name: "hybrid",
			mode: ExposureHybrid,
			selections: []ToolBinding{
				mustBinding(t, first, true),
				mustBinding(t, second, false),
			},
			wantDirect: []catalogdomain.ToolID{first},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			profile, err := NewProfileWithBindings(id, slug, "Default", tc.mode, tc.selections)
			if err != nil {
				t.Fatalf("NewProfileWithBindings() error = %v", err)
			}
			if !slices.Equal(profile.DirectToolIDs(), tc.wantDirect) {
				t.Fatalf("DirectToolIDs() = %v, want %v", profile.DirectToolIDs(), tc.wantDirect)
			}
			if !profile.Contains(first) || !profile.Contains(second) {
				t.Fatal("Profile.Contains() rejected selected Tool")
			}
		})
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
		ExposureHybrid,
		[]ToolBinding{
			mustBinding(t, toolID, true),
			mustBinding(t, toolID, false),
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

func mustBinding(t *testing.T, toolID catalogdomain.ToolID, direct bool) ToolBinding {
	t.Helper()
	binding, err := NewToolBinding(ToolBindingSpec{ToolID: toolID, Direct: direct})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}
