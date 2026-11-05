package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func deliveryFixture(t *testing.T) (*Runtime, context.Context, string, profiledomain.Profile) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(root, "test.db"),
		SkillRoot:    filepath.Join(root, "skill-store"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	p, err := createMCPProfileFixture(t, r, ctx, profileapp.CreateProfileRequest{
		Slug: "deliver", DisplayName: "Deliver",
	}, "hybrid")

	if err != nil {
		t.Fatal(err)
	}
	return r, ctx, root, p
}

func enabledTestSkill(t *testing.T, r *Runtime, ctx context.Context, source string) skilldomain.InstallationID {
	t.Helper()
	item, err := r.Skills.InstallLocal(ctx, writeRuntimeSkill(t, source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Skills.SetTrust(ctx, item.ID(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Skills.SetEnabled(ctx, item.ID(), true); err != nil {
		t.Fatal(err)
	}
	return item.ID()
}

func selectTestSkills(t *testing.T, r *Runtime, ctx context.Context, slug profiledomain.Slug, ids ...skilldomain.InstallationID) profiledomain.Profile {
	t.Helper()
	p, err := r.Profiles.ReplaceSkills.Execute(ctx, profileapp.ReplaceSkillsRequest{
		ProfileSlug: slug, InstallationIDs: ids,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProfileSkillDeliveryOwnsOnlyExplicitSelectionsAndLeavesLegacyUnassigned(t *testing.T) {
	r, ctx, root, p := deliveryFixture(t)
	skills, err := r.Skills.List(ctx)
	if err != nil || len(skills) == 0 {
		t.Fatalf("builtins %v %v", skills, err)
	}
	builtin := skills[0].InstallationID
	custom := enabledTestSkill(t, r, ctx, filepath.Join(root, "custom-skill"))
	p = selectTestSkills(t, r, ctx, p.Slug(), builtin, custom)
	legacyRoot := filepath.Join(root, "legacy")
	ownedRoot := filepath.Join(root, "owned")
	for _, dir := range []string{legacyRoot, ownedRoot} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := r.Skills.Deploy(ctx, builtin, skilldomain.DeploymentAgentsProject, legacyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.OwnerProfileID() != "" || legacy.ProfileRevision() != 0 {
		t.Fatalf("legacy wrongly assigned: %+v", legacy)
	}

	deployed, err := r.DeployProfileSkills(ctx, p.Slug(), skilldomain.DeploymentAgentsProject, ownedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(deployed) != 2 {
		t.Fatalf("Profile delivery len = %d, want 2", len(deployed))
	}
	for _, item := range deployed {
		if item.Deployment.OwnerProfileID() != p.ID().String() ||
			item.Deployment.ProfileRevision() != p.Revision() ||
			item.Drift != skilldomain.DeploymentCurrent {
			t.Fatalf("owned deployment = %+v", item)
		}
		if _, err := os.Stat(item.Deployment.Destination()); err != nil {
			t.Fatal(err)
		}
	}
	owned, err := r.ListProfileSkillDeployments(ctx, p.Slug())
	if err != nil || len(owned) != 2 {
		t.Fatalf("owned list = %v %v", owned, err)
	}
	for _, item := range owned {
		if item.Deployment.OwnerProfileID() != p.ID().String() {
			t.Fatalf("owner mismatch: %+v", item)
		}
	}
	second, err := createMCPProfileFixture(t, r, ctx, profileapp.CreateProfileRequest{
		Slug: "another", DisplayName: "Another",
	}, "hybrid")

	if err != nil {
		t.Fatal(err)
	}
	other, err := r.ListProfileSkillDeployments(ctx, second.Slug())
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-Profile delivery leak = %v %v", other, err)
	}
	all, err := r.Skills.ListDeployments(ctx)
	if err != nil || len(all) != 3 {
		t.Fatalf("all list = %v %v", all, err)
	}
	legacyFound := false
	for _, item := range all {
		if item.Deployment.ID() == legacy.ID() {
			legacyFound = item.Deployment.OwnerProfileID() == ""
		}
	}
	if !legacyFound {
		t.Fatal("legacy delivery auto-claimed or disappeared")
	}

	// No silent overwrites; existing owned deliveries remain intact on conflict.
	if _, err := r.DeployProfileSkills(ctx, p.Slug(), skilldomain.DeploymentAgentsProject, ownedRoot); err == nil {
		t.Fatal("expected exact deployment target conflict")
	}
	all, err = r.Skills.ListDeployments(ctx)
	if err != nil || len(all) != 3 {
		t.Fatalf("conflict changed deployment inventory: %d %v", len(all), err)
	}

	// Profile revisions change independently of already materialized files.
	changed := selectTestSkills(t, r, ctx, p.Slug(), builtin)
	if changed.Revision() == p.Revision() {
		t.Fatal("revision did not advance")
	}
	owned, err = r.ListProfileSkillDeployments(ctx, p.Slug())
	if err != nil || len(owned) != 2 || owned[0].Deployment.ProfileRevision() != p.Revision() {
		t.Fatalf("prior Profile revision not retained: %v %v", owned, err)
	}
	// Managed content is immutable/read-only until explicitly uninstalled.
	// Tests must invoke the legitimate cleanup path instead of unlinking it.
	for _, item := range all {
		if err := r.Skills.Undeploy(ctx, item.Deployment.ID()); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Skills.Remove(ctx, custom); err != nil {
		t.Fatal(err)
	}
}

func TestProfileSkillDeliveryRollsBackOnlyNewSnapshotsOnLateTargetConflict(t *testing.T) {
	r, ctx, root, p := deliveryFixture(t)
	first := enabledTestSkill(t, r, ctx, filepath.Join(root, "source", "first-skill"))
	second := enabledTestSkill(t, r, ctx, filepath.Join(root, "source", "second-skill"))
	p = selectTestSkills(t, r, ctx, p.Slug(), first, second)
	sorted := p.Skills()
	if len(sorted) != 2 {
		t.Fatalf("selection = %v", sorted)
	}
	name := map[skilldomain.InstallationID]string{
		first: "first-skill", second: "second-skill",
	}
	project := filepath.Join(root, "project")
	parent := filepath.Join(project, ".agents", "skills")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	// Pre-create the SECOND exact destination, forcing rollback of the first.
	collision := filepath.Join(parent, name[sorted[1]])
	if err := os.Mkdir(collision, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(collision, "existing.txt")
	if err := os.WriteFile(marker, []byte("owner"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DeployProfileSkills(ctx, p.Slug(), skilldomain.DeploymentAgentsProject, project); err == nil {
		t.Fatal("expected late destination collision")
	}
	if _, err := os.Stat(filepath.Join(parent, name[sorted[0]])); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first materialization not rolled back: %v", err)
	}
	//nolint:gosec // Marker path is inside the isolated test fixture TempDir.
	if raw, err := os.ReadFile(marker); err != nil || string(raw) != "owner" {
		t.Fatalf("pre-existing target modified: %q %v", raw, err)
	}
	owned, err := r.ListProfileSkillDeployments(ctx, p.Slug())
	if err != nil || len(owned) != 0 {
		t.Fatalf("rollback left metadata: %v %v", owned, err)
	}
	selectTestSkills(t, r, ctx, p.Slug())
	for _, id := range []skilldomain.InstallationID{first, second} {
		if err := r.Skills.Remove(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProfileSkillDeliveryRejectsDuplicateTargetNamesAndStaleRevision(t *testing.T) {
	r, ctx, root, p := deliveryFixture(t)
	id1 := enabledTestSkill(t, r, ctx, filepath.Join(root, "a", "same-skill"))
	id2 := enabledTestSkill(t, r, ctx, filepath.Join(root, "b", "same-skill"))
	p = selectTestSkills(t, r, ctx, p.Slug(), id1, id2)
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DeployProfileSkills(ctx, p.Slug(), skilldomain.DeploymentAgentsProject, project); err == nil {
		t.Fatal("two versions with one target name accepted")
	}
	if _, err := os.Stat(filepath.Join(project, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight wrote directories: %v", err)
	}
	// The SQLite reference admission trigger also rejects stale/synthetic
	// ownership even when the direct Skill Deployment adapter is invoked.
	p = selectTestSkills(t, r, ctx, p.Slug(), id1)
	if _, err := r.Skills.deployForProfile(ctx, id1, skilldomain.DeploymentAgentsProject,
		project, p.ID().String(), p.Revision()+1); err == nil {
		t.Fatal("stale Profile revision admitted at database boundary")
	}
	all, err := r.Skills.ListDeployments(ctx)
	if err != nil || len(all) != 0 {
		t.Fatalf("rejected delivery left metadata: %v %v", all, err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "same-skill")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected delivery left directory: %v", err)
	}
	// Profile selection remains exact and ordered.
	if !slices.Equal(p.Skills(), []skilldomain.InstallationID{id1}) {
		t.Fatalf("selection = %v", p.Skills())
	}
	selectTestSkills(t, r, ctx, p.Slug())
	for _, id := range []skilldomain.InstallationID{id1, id2} {
		if err := r.Skills.Remove(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
}
