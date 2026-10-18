package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
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
	for _, action := range []string{"read", "edit", "shell", "execute"} {
		if got[action] != "ask" {
			t.Fatalf("OpenCode native %s not reviewed: %+v", action, got)
		}
	}
	for _, action := range []string{"external_directory", "subagent"} {
		if got[action] != "deny" {
			t.Fatalf("OpenCode %s not denied: %+v", action, got)
		}
	}
}

func TestOpenCodeRestrictedWithoutToolSupplyStillRequiresOwnerExecuteApproval(t *testing.T) {
	factory, err := NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Availability(agentdomain.BackendOpenCode); errors.Is(err, agentdomain.ErrUnavailable) {
		t.Skip("OpenCode not installed in trusted locations on this host")
	} else if err != nil {
		t.Fatal(err)
	}
	spec := fixtureAgent(t).Spec()
	spec.Backend = agentdomain.BackendOpenCode
	spec.PluginID = agentdomain.BackendOpenCode.PluginID()
	spec.Isolation = "restricted_host"
	agent, err := agentdomain.NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	// Ungoverned restricted execution: no ToolSupply, ordinary review rules
	// still apply and project config disabling must NOT be injected.
	cmd, err := factory.CommandInWorkspace(context.Background(), agent, spec.WorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	var suppliedPolicy string
	for _, v := range cmd.Env {
		if strings.HasPrefix(v, "OPENCODE_CONFIG_CONTENT=") {
			suppliedPolicy = strings.TrimPrefix(v, "OPENCODE_CONFIG_CONTENT=")
		}
		if v == "OPENCODE_DISABLE_PROJECT_CONFIG=1" {
			t.Fatal("ungoverned restricted command must not disable project config")
		}
	}
	if suppliedPolicy == "" {
		t.Fatal("restricted command started without owner review rules")
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
	for _, action := range []string{"read", "edit", "shell", "execute"} {
		if got[action] != "ask" {
			t.Fatalf("OpenCode native %s not requiring owner approval: %+v", action, got)
		}
	}
	for _, action := range []string{"external_directory", "subagent"} {
		if got[action] != "deny" {
			t.Fatalf("OpenCode %s not denied: %+v", action, got)
		}
	}
}

func TestAllAgentBackendsRejectHostGovernedToolSupply(t *testing.T) {
	factory, err := NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	supply := agentapplication.ToolSupply{
		Delivery:    agentapplication.ToolDeliveryMCPHTTP,
		Endpoint:    "http://127.0.0.1:12345/mcp",
		BearerToken: strings.Repeat("a", 64),
	}
	for _, backend := range []agentdomain.Backend{agentdomain.BackendGrok, agentdomain.BackendOpenCode, agentdomain.BackendCodex} {
		t.Run(string(backend), func(t *testing.T) {
			spec := fixtureAgent(t).Spec()
			spec.Backend = backend
			spec.PluginID = backend.PluginID()
			spec.Isolation = "host"
			agent, err := agentdomain.NewAgent(spec)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := factory.CommandWithConfig(context.Background(), agent,
				agentapplication.RunConfig{WorkspaceRoot: spec.WorkspaceRoot, ToolSupply: supply}); !errors.Is(err, agentdomain.ErrUnavailable) {
				t.Fatalf("host backend %s received governed Tool credentials: %v", backend, err)
			}
		})
	}
}
func TestRestrictedAgentCommandPlansReadOnlyProjectForTaskTools(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Seatbelt plan acceptance")
	}
	factory, err := NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if !processisolation.New().Info().RestrictedHostAvailable {
		t.Skip("no restricted-host execution boundary available")
	}
	for _, backend := range []agentdomain.Backend{agentdomain.BackendOpenCode, agentdomain.BackendGrok} {
		t.Run(string(backend), func(t *testing.T) {
			if _, err := factory.Availability(backend); errors.Is(err, agentdomain.ErrUnavailable) {
				t.Skip("Agent CLI not installed in trusted locations")
			} else if err != nil {
				t.Fatal(err)
			}
			spec := fixtureAgent(t).Spec()
			spec.Backend = backend
			spec.PluginID = backend.PluginID()
			spec.Isolation = "restricted_host"
			agent, err := agentdomain.NewAgent(spec)
			if err != nil {
				t.Fatal(err)
			}
			supply := agentapplication.ToolSupply{Delivery: agentapplication.ToolDeliveryMCPHTTP,
				Endpoint: "http://127.0.0.1:12345/mcp", BearerToken: strings.Repeat("a", 64)}
			cmd, err := factory.CommandWithConfig(context.Background(), agent,
				agentapplication.RunConfig{WorkspaceRoot: spec.WorkspaceRoot, ToolSupply: supply})
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != "/usr/bin/sandbox-exec" || len(cmd.Args) < 4 || cmd.Args[1] != "-p" {
				t.Fatalf("unconfined native Agent command plan: %+v", cmd.Args)
			}
			policy := cmd.Args[2]
			if !strings.Contains(policy, "(deny default)") ||
				strings.Contains(policy, "(allow file-write* (literal "+strconv.Quote(spec.WorkspaceRoot)+")") {
				t.Fatalf("governed Tool supply permitted native project writes: %s", policy)
			}
		})
	}
}
