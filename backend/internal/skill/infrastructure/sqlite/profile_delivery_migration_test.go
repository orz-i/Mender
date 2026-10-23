package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestUpgradeSkillDeploymentV2RetainsLegacyUnassignedRecords(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	initial, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	row := testDeployment(t,
		"sdep_00112233445566778899aabbccddeeff",
		"sinst_00112233445566778899aabbccddeeff",
		skilldomain.DeploymentAgentsProject,
		root, filepath.Join(root, ".agents", "skills", "legacy"),
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		time.Now().UTC(),
	)
	if err := initial.CreateDeployment(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}

	// Construct a faithful v2 column shape in a disposable database.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DROP INDEX idx_skill_deployments_owner",
		"ALTER TABLE skill_deployments DROP COLUMN owner_profile_id",
		"ALTER TABLE skill_deployments DROP COLUMN profile_revision",
		"DELETE FROM skill_installation_schema_migrations WHERE version = 3",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			t.Fatalf("prepare v2: %s: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	legacy, err := upgraded.GetDeployment(ctx, row.ID())
	if err != nil {
		t.Fatal(err)
	}
	if legacy.OwnerProfileID() != "" || legacy.ProfileRevision() != 0 ||
		legacy.InstallationID() != row.InstallationID() ||
		legacy.Destination() != row.Destination() {
		t.Fatalf("v2 deployment altered by migration: %+v", legacy)
	}
	owned, err := upgraded.ListDeploymentsByProfile(ctx, "prof_00000000000000000000000000000001")
	if err != nil || len(owned) != 0 {
		t.Fatalf("legacy auto-claimed: %+v %v", owned, err)
	}
	var version int
	if err := upgraded.db.QueryRowContext(ctx,
		"SELECT MAX(version) FROM skill_installation_schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("migration version = %d, want 3", version)
	}
}

func TestProfileOwnedSkillDeploymentRoundTripAndExactOwnerFiltering(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
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
