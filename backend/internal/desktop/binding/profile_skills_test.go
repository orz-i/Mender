package binding

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestWailsProfileSkillsRoundTripWithoutHostDeployment(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	service := NewService(ctx, runtime)
	created, err := service.CreateProfile("delivery", "Delivery", "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Skills) != 0 {
		t.Fatalf("initial skills = %v", created.Skills)
	}
	inventory, err := runtime.Skills.List(ctx)
	if err != nil || len(inventory) == 0 {
		t.Fatalf("skill inventory: %v, %v", inventory, err)
	}
	installationID := inventory[0].InstallationID.String()
	updated, err := service.ReplaceProfileSkills("delivery", []string{installationID})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Skills) != 1 || updated.Skills[0] != installationID {
		t.Fatalf("updated Profile Skills = %v", updated.Skills)
	}
	loaded, err := service.GetProfile("delivery")
	if err != nil || len(loaded.Skills) != 1 || loaded.Skills[0] != installationID {
		t.Fatalf("GetProfile Skills = %v, %v", loaded.Skills, err)
	}
	if _, err := service.ReplaceProfileSkills("delivery", []string{"invalid"}); err == nil {
		t.Fatal("invalid Skill ID accepted")
	}
	if _, err := service.ReplaceProfileSkills("delivery", nil); err != nil {
		t.Fatal(err)
	}
	deployments, err := runtime.Skills.ListDeployments(ctx)
	if err != nil || len(deployments) != 0 {
		t.Fatalf("Skill selection materialized unexpected deployment: %v, %v", deployments, err)
	}
}
