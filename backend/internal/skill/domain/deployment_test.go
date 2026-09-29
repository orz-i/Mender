package domain

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDeploymentValidation(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	id, err := ParseDeploymentID("sdep_0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	installationID, err := ParseInstallationID("sinst_0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	item, err := NewDeployment(
		id,
		installationID,
		DeploymentAgentsProject,
		root,
		filepath.Join(root, ".agents", "skills", "example-skill"),
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewDeployment() error = %v", err)
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if _, err := NewDeployment(
		id,
		installationID,
		DeploymentAgentsProject,
		root,
		root,
		item.Digest(),
		item.DeployedAt(),
	); err == nil {
		t.Fatal("NewDeployment(root destination) error = nil")
	}
	if err := DeploymentTarget("unknown").Validate(); err == nil {
		t.Fatal("DeploymentTarget(unknown).Validate() error = nil")
	}
}

func TestDeploymentTargetScope(t *testing.T) {
	t.Parallel()

	if !DeploymentAgentsProject.ProjectScoped() ||
		!DeploymentGitHubProject.ProjectScoped() ||
		!DeploymentClaudeProject.ProjectScoped() {
		t.Fatal("project target ProjectScoped() = false")
	}
	if DeploymentAgentsUser.ProjectScoped() || DeploymentCopilotUser.ProjectScoped() {
		t.Fatal("user target ProjectScoped() = true")
	}
}
