package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestProfileOwnedSkillDeploymentRoundTripAndExactOwnerFiltering(t *testing.T) {
	ctx := context.Background()
	store, err := openSkillV3(t, ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	first := testDeployment(t,
		"sdep_00112233445566778899aabbccddeeff",
		"sinst_00112233445566778899aabbccddeeff",
		skilldomain.DeploymentAgentsProject,
		root, filepath.Join(root, ".agents", "skills", "first"),
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		time.Now().UTC(),
	)
	first, err = first.WithProfile("prof_00000000000000000000000000000001", 4)
	if err != nil {
		t.Fatal(err)
	}
	// The current v3 guard requires a reviewed, enabled Skill selection at
	// the exact Profile revision. No legacy unassigned row may bypass it.
	inst := testInstallation(t, first.InstallationID().String(), "skill_00112233445566778899aabbccddeeff", "first", first.Digest(), time.Now().UTC())
	if err := store.Create(ctx, inst); err != nil {
		t.Fatal(err)
	}
	if err := store.SetState(ctx, inst.ID(), true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO profiles(id,slug,display_name,revision) VALUES(?,?,?,?)`, first.OwnerProfileID(), "reviewed-skill", "Reviewed Skill", 4); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO profile_skills(profile_id,installation_id) VALUES(?,?)`, first.OwnerProfileID(), first.InstallationID().String()); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDeployment(ctx, first); err != nil {
		t.Fatal(err)
	}
	legacy := testDeployment(t,
		"sdep_11112222333344445555666677778888",
		"sinst_11112222333344445555666677778888",
		skilldomain.DeploymentAgentsProject,
		root, filepath.Join(root, ".agents", "skills", "legacy"),
		first.Digest(), time.Now().UTC(),
	)
	if err := store.CreateDeployment(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListDeploymentsByProfile(ctx, first.OwnerProfileID())
	if err != nil || len(list) != 1 || list[0].ProfileRevision() != 4 ||
		list[0].ID() != first.ID() {
		t.Fatalf("owner list = %+v error=%v", list, err)
	}
	other, err := store.ListDeploymentsByProfile(ctx, "prof_11111111111111111111111111111111")
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-owner entries: %v %v", other, err)
	}
}
