package sqlite

import (
	"context"
	"errors"
	"slices"
	"testing"

	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestProfileSkillMembershipRoundTripPreservesOtherSettingsAndCAS(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	created := auditProfile(t, store)
	first, _ := skilldomain.ParseInstallationID("sinst_00000000000000000000000000000001")
	second, _ := skilldomain.ParseInstallationID("sinst_00000000000000000000000000000002")

	original, err := store.GetProfileByID(ctx, created.ID())
	if err != nil {
		t.Fatal(err)
	}
	changed, err := original.ReplaceSkills([]skilldomain.InstallationID{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, changed); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetProfileBySlug(ctx, created.Slug())
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision() != 1 || !slices.Equal(got.Skills(), []skilldomain.InstallationID{first, second}) {
		t.Fatalf("skills and revision = %v, %d", got.Skills(), got.Revision())
	}
	usages, err := store.ListSkillProfiles(ctx, first)
	if err != nil || !slices.Equal(usages, []string{"audit-profile"}) {
		t.Fatalf("profile skill usages = %v, %v", usages, err)
	}

	// A Tool or Tunnel mutation must not erase independent Skill membership.
	other, err := got.ReplaceBindings(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, other); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.GetProfileByID(ctx, created.ID())
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Revision() != 2 || !slices.Equal(reloaded.Skills(), []skilldomain.InstallationID{first, second}) {
		t.Fatalf("tool update discarded skills: %v, revision=%d", reloaded.Skills(), reloaded.Revision())
	}

	stale, err := got.ReplaceSkills(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, stale); !errors.Is(err, profileapplication.ErrRevisionConflict) {
		t.Fatalf("stale skill mutation = %v, want revision conflict", err)
	}
	cleared, err := reloaded.ReplaceSkills(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, cleared); err != nil {
		t.Fatal(err)
	}
	after, err := store.ListProfiles(ctx)
	if err != nil || len(after) != 1 || len(after[0].Skills()) != 0 || after[0].Revision() != 3 {
		t.Fatalf("cleared profile = %v, %v", after, err)
	}
	usages, err = store.ListSkillProfiles(ctx, first)
	if err != nil || len(usages) != 0 {
		t.Fatalf("removed skill still in use: %v, %v", usages, err)
	}
}
