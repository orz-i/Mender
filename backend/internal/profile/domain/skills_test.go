package domain

import (
	"slices"
	"testing"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestProfileSkillMembershipImmutableExactAndPreservedDuringToolMutation(t *testing.T) {
	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("delivery")
	profile, err := NewProfileWithBindings(id, slug, "Delivery", ExposureHybrid, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := skilldomain.ParseInstallationID("sinst_00000000000000000000000000000001")
	second, _ := skilldomain.ParseInstallationID("sinst_00000000000000000000000000000002")
	input := []skilldomain.InstallationID{second, first}
	updated, err := profile.ReplaceSkills(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = first
	if !slices.Equal(updated.Skills(), []skilldomain.InstallationID{first, second}) || len(profile.Skills()) != 0 {
		t.Fatalf("mutable or unsorted selection: %v, original: %v", updated.Skills(), profile.Skills())
	}
	direct := updated.Skills()
	direct[0] = second
	if updated.Skills()[0] != first {
		t.Fatal("Skills exposed mutable slice")
	}
	if _, err := updated.ReplaceSkills([]skilldomain.InstallationID{first, first}); err == nil {
		t.Fatal("duplicate installation accepted")
	}
	if _, err := updated.ReplaceSkills([]skilldomain.InstallationID{""}); err == nil {
		t.Fatal("invalid installation accepted")
	}
	next, err := updated.ReplaceBindings(nil)
	if err != nil || !slices.Equal(next.Skills(), []skilldomain.InstallationID{first, second}) {
		t.Fatalf("tool change lost selected Skills: %v, %v", next.Skills(), err)
	}
}
