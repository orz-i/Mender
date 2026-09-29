package deploymentfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
)

func TestResolveDestinationUsesSupportedHostLayout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := New()

	tests := []struct {
		target skilldomain.DeploymentTarget
		want   string
	}{
		{skilldomain.DeploymentAgentsProject, filepath.Join(root, ".agents", "skills", "example-skill")},
		{skilldomain.DeploymentGitHubProject, filepath.Join(root, ".github", "skills", "example-skill")},
		{skilldomain.DeploymentClaudeProject, filepath.Join(root, ".claude", "skills", "example-skill")},
		{skilldomain.DeploymentAgentsUser, filepath.Join(root, ".agents", "skills", "example-skill")},
		{skilldomain.DeploymentCopilotUser, filepath.Join(root, ".copilot", "skills", "example-skill")},
	}
	for _, test := range tests {
		got, err := store.ResolveDestination(test.target, root, "example-skill")
		if err != nil {
			t.Fatalf("ResolveDestination(%s) error = %v", test.target, err)
		}
		if got != test.want {
			t.Fatalf("ResolveDestination(%s) = %q, want %q", test.target, got, test.want)
		}
		parent := filepath.Dir(got)
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("deployment parent %q info=%v err=%v", parent, info, err)
		}
	}
}

func TestResolveDestinationRejectsSymlinkedParentsAndExistingTarget(t *testing.T) {
	t.Parallel()

	store := New()

	t.Run("symlink parent", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, ".agents")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		_, err := store.ResolveDestination(
			skilldomain.DeploymentAgentsProject,
			root,
			"example-skill",
		)
		if err == nil || !strings.Contains(err.Error(), "must be a real directory") {
			t.Fatalf("ResolveDestination() error = %v, want symlink rejection", err)
		}
	})

	t.Run("existing target", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		destination, err := store.ResolveDestination(
			skilldomain.DeploymentAgentsProject,
			root,
			"example-skill",
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(destination, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err = store.ResolveDestination(
			skilldomain.DeploymentAgentsProject,
			root,
			"example-skill",
		)
		if err == nil || !strings.Contains(err.Error(), "target already exists") {
			t.Fatalf("ResolveDestination(existing) error = %v", err)
		}
	})
}

func TestObserveAndRemoveTrackDeploymentDrift(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := New()
	root := t.TempDir()
	destination, err := store.ResolveDestination(
		skilldomain.DeploymentAgentsProject,
		root,
		"example-skill",
	)
	if err != nil {
		t.Fatal(err)
	}
	writeDeploymentSkill(t, destination, "initial")
	snapshot, err := agentskill.LoadDirectory(destination)
	if err != nil {
		t.Fatal(err)
	}

	exists, digest, err := store.Observe(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || digest != snapshot.Digest {
		t.Fatalf("Observe() exists=%v digest=%q want=%q", exists, digest, snapshot.Digest)
	}

	if err := os.WriteFile(
		filepath.Join(destination, "SKILL.md"),
		[]byte("---\nname: example-skill\ndescription: changed\n---\n# changed\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	exists, changedDigest, err := store.Observe(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || changedDigest == "" || changedDigest == digest {
		t.Fatalf("Observe(changed) exists=%v digest=%q original=%q", exists, changedDigest, digest)
	}

	if err := store.Remove(ctx, root, destination); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	exists, digest, err = store.Observe(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if exists || digest != "" {
		t.Fatalf("Observe(removed) exists=%v digest=%q", exists, digest)
	}
}

func writeDeploymentSkill(t *testing.T, root, body string) {
	t.Helper()
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: example-skill\ndescription: Deployment adapter test.\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
