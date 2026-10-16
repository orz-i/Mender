package acp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

func TestOpenCodeLocalLaunchUsesPinnedExecutableAndACPStdioOnly(t *testing.T) {
	factory, err := NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	// Verify that PATH injection is never allowed to control Agent binaries.
	fake := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, filepath.Join(fake, "opencode")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fake)
	binary, err := factory.Availability(agentdomain.BackendOpenCode)
	if errors.Is(err, agentdomain.ErrUnavailable) {
		t.Skip("OpenCode not installed in trusted locations on this host")
	}
	if err != nil {
		t.Fatal(err)
	}
	if binary == filepath.Join(fake, "opencode") {
		t.Fatal("untrusted PATH executable selected")
	}
	agentSpec := fixtureAgent(t).Spec()
	canonicalRoot, err := filepath.EvalSymlinks(agentSpec.WorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	agentSpec.WorkspaceRoot = canonicalRoot
	agentSpec.Backend = agentdomain.BackendOpenCode
	agentSpec.PluginID = agentdomain.BackendOpenCode.PluginID()
	agentSpec.Isolation = "host" // test only constructs process plan, does not execute
	a, err := agentdomain.NewAgent(agentSpec)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd, err := factory.Command(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != binary || !slices.Equal(cmd.Args, []string{binary, "acp"}) ||
		cmd.Dir != a.Spec().WorkspaceRoot {
		t.Fatalf("unreviewed OpenCode invocation: path=%q args=%q cwd=%q", cmd.Path, cmd.Args, cmd.Dir)
	}
	for _, v := range cmd.Env {
		if v == "CI=1" || v == "NO_BROWSER=1" {
			continue
		}
		if len(v) >= 13 && v[:13] == "OPENCODE_API_" {
			t.Fatalf("inherited sensitive environment name: %q", v[:13])
		}
	}
}
