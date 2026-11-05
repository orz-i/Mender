package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestProfileSelectsOnlyTrustedEnabledExactSkillsAndGuardsRemoval(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(root, "mender.db"), SkillRoot: filepath.Join(root, "managed"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	profile, err := createMCPProfileFixture(t, runtime, ctx, profileapplication.CreateProfileRequest{
		Slug: "delivery", DisplayName: "Delivery",
	}, "hybrid")

	if err != nil {
		t.Fatal(err)
	}
	managed, err := runtime.Skills.InstallLocal(ctx, writeRuntimeSkill(t, filepath.Join(root, "example-skill")))
	if err != nil {
		t.Fatal(err)
	}

	replace := func(ids ...skilldomain.InstallationID) (profiledomain.Profile, error) {
		return runtime.Profiles.ReplaceSkills.Execute(ctx, profileapplication.ReplaceSkillsRequest{
			ProfileSlug: profile.Slug(), InstallationIDs: ids,
		})
	}
	if _, err := replace(managed.ID()); !errors.Is(err, profileapplication.ErrSkillNotDeployable) {
		t.Fatalf("untrusted installation accepted: %v", err)
	}
	if _, err := runtime.Skills.SetTrust(ctx, managed.ID(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := replace(managed.ID()); !errors.Is(err, profileapplication.ErrSkillNotDeployable) {
		t.Fatalf("disabled installation accepted: %v", err)
	}
	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := replace(managed.ID(), managed.ID()); !errors.Is(err, profileapplication.ErrInvalidSkillMembership) {
		t.Fatalf("duplicate skill accepted: %v", err)
	}
	chosen, err := replace(managed.ID())
	if err != nil {
		t.Fatal(err)
	}
	if chosen.Revision() != 1 || len(chosen.Skills()) != 1 || chosen.Skills()[0] != managed.ID() {
		t.Fatalf("selection = %v rev=%d", chosen.Skills(), chosen.Revision())
	}
	// An independent SQLite client must not bypass the application-layer
	// reference check while an exact Profile selection is active.
	raw, err := sql.Open("sqlite", filepath.Join(root, "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, "UPDATE skill_installations SET enabled=0 WHERE installation_id=?", managed.ID().String()); err == nil {
		t.Fatal("independent DB connection bypassed Profile Skill reference guard")
	}
	if _, err := raw.ExecContext(ctx, "DELETE FROM skill_installations WHERE installation_id=?", managed.ID().String()); err == nil {
		t.Fatal("independent DB connection removed an installation still bound to Profile")
	}
	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), false); !errors.Is(err, skillapplication.ErrInstallationInUse) {
		t.Fatalf("disabled linked installation: %v", err)
	}
	if _, err := runtime.Skills.SetTrust(ctx, managed.ID(), false); !errors.Is(err, skillapplication.ErrInstallationInUse) {
		t.Fatalf("untrusted linked installation: %v", err)
	}
	if err := runtime.Skills.Remove(ctx, managed.ID()); !errors.Is(err, skillapplication.ErrInstallationInUse) {
		t.Fatalf("deleted linked installation: %v", err)
	}
	if _, err := replace(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), false); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Skills.Remove(ctx, managed.ID()); err != nil {
		t.Fatal(err)
	}
}
