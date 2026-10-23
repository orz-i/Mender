package binding

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestDesktopDeploysOnlySelectedProfileSkillsToExplicitProjectRoot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(root, "state.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	s := NewService(ctx, r)
	profile, err := s.CreateProfile("local-skill-delivery", "Local Skills", "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	list, err := r.Skills.List(ctx)
	if err != nil || len(list) < 1 {
		t.Fatalf("Skill inventory = %v %v", list, err)
	}
	selected, err := s.ReplaceProfileSkills(profile.Slug, []string{list[0].InstallationID.String()})
	if err != nil {
		t.Fatal(err)
	}

	projectRoot := filepath.Join(root, "project")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeployProfileSkills(profile.Slug, "agents-project", ""); err == nil {
		t.Fatal("project target accepted without project root")
	}
	if _, err := s.DeployProfileSkills(profile.Slug, "agents-user", projectRoot); err == nil {
		t.Fatal("user target accepted a project root")
	}
	created, err := s.DeployProfileSkills(profile.Slug, "agents-project", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Deployments) != 1 ||
		created.Deployments[0].ProfileID != profile.ProfileID ||
		created.Deployments[0].ProfileRevision != selected.Revision {
		t.Fatalf("Profile delivery = %#v", created)
	}
	if _, err := os.Stat(created.Deployments[0].Destination); err != nil {
		t.Fatal(err)
	}
	owned, err := s.ListProfileSkillDeployments(profile.Slug)
	if err != nil || len(owned.Deployments) != 1 {
		t.Fatalf("Profile deliveries = %v, %v", owned, err)
	}
	if err := s.UndeploySkill(created.Deployments[0].DeploymentID); err == nil {
		t.Fatal("legacy undeploy bypassed Profile ownership")
	}
	if err := s.UndeployProfileSkill("different", created.Deployments[0].DeploymentID); err == nil {
		t.Fatal("other Profile removed owned Skill deployment")
	}
	if err := s.UndeployProfileSkill(profile.Slug, created.Deployments[0].DeploymentID); err != nil {
		t.Fatal(err)
	}
	owned, err = s.ListProfileSkillDeployments(profile.Slug)
	if err != nil || len(owned.Deployments) != 0 {
		t.Fatalf("Undeploy = %v %v", owned, err)
	}
}
