package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	var suppliedPolicy string
	for _, v := range cmd.Env {
		if strings.HasPrefix(v, "OPENCODE_CONFIG_CONTENT=") {
			suppliedPolicy = strings.TrimPrefix(v, "OPENCODE_CONFIG_CONTENT=")
		}
		if v == "CI=1" || v == "NO_BROWSER=1" {
			continue
		}
		if len(v) >= 13 && v[:13] == "OPENCODE_API_" {
			t.Fatalf("inherited sensitive environment name: %q", v[:13])
		}
	}
	if suppliedPolicy == "" {
		t.Fatal("OpenCode was started without owner-mediated V2 review rules")
	}
	var cfg struct {
		Permissions []struct {
			Action   string `json:"action"`
			Resource string `json:"resource"`
			Effect   string `json:"effect"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(suppliedPolicy), &cfg); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, rule := range cfg.Permissions {
		if rule.Resource != "*" {
			t.Fatalf("non-universal review rule: %+v", rule)
		}
		got[rule.Action] = rule.Effect
	}
	for _, action := range []string{"read", "edit", "shell"} {
		if got[action] != "ask" {
			t.Fatalf("OpenCode native %s not reviewed: %+v", action, got)
		}
	}
	for _, action := range []string{"external_directory", "subagent", "execute"} {
		if got[action] != "deny" {
			t.Fatalf("OpenCode %s not denied: %+v", action, got)
		}
	}
}
