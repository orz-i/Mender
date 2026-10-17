package acp

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

func TestOpenCodeGovernedToolSupplyUsesNarrowV2CodeModePolicy(t *testing.T) {
	if runtime.GOOS != "darwin" || !processisolation.New().Info().RestrictedHostAvailable {
		t.Skip("native macOS governed OpenCode process plan")
	}
	factory, err := NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Availability(agentdomain.BackendOpenCode); err != nil {
		if errors.Is(err, agentdomain.ErrUnavailable) {
			t.Skip("OpenCode executable unavailable")
		}
		t.Fatal(err)
	}
	spec := fixtureAgent(t).Spec()
	spec.Backend = agentdomain.BackendOpenCode
	spec.PluginID = agentdomain.BackendOpenCode.PluginID()
	spec.Isolation = "restricted_host"
	a, err := agentdomain.NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	supply := agentapp.ToolSupply{Delivery: agentapp.ToolDeliveryMCPHTTP,
		Endpoint: "http://127.0.0.1:12345/mcp", BearerToken: strings.Repeat("a", 64)}
	cmd, err := factory.CommandWithConfig(context.Background(), a, agentapp.RunConfig{
		WorkspaceRoot: spec.WorkspaceRoot, ToolSupply: supply,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != "/usr/bin/sandbox-exec" || !strings.Contains(cmd.Args[2], "(deny default)") ||
		strings.Contains(cmd.Args[2], "(allow file-write* (literal "+strconv.Quote(spec.WorkspaceRoot)+")") {
		t.Fatal("governed OpenCode lacks restricted read-only project placement")
	}
	if !slices.Contains(cmd.Env, "OPENCODE_DISABLE_PROJECT_CONFIG=1") {
		t.Fatal("governed OpenCode can load project-supplied config/plugins")
	}
	var content string
	for _, entry := range cmd.Env {
		if strings.Contains(entry, supply.BearerToken) {
			t.Fatal("Task bearer leaked into environment before ACP negotiation")
		}
		if strings.HasPrefix(entry, "OPENCODE_CONFIG_CONTENT=") {
			if content != "" {
				t.Fatal("multiple OpenCode policy overlays")
			}
			content = strings.TrimPrefix(entry, "OPENCODE_CONFIG_CONTENT=")
		}
	}
	var config map[string][]map[string]string
	if err := json.Unmarshal([]byte(content), &config); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, rule := range config["permissions"] {
		if rule["resource"] != "*" || got[rule["action"]] != "" {
			t.Fatal("duplicate or malformed governed OpenCode policy rule")
		}
		got[rule["action"]] = rule["effect"]
	}
	for action, expected := range map[string]string{
		"read": "ask", "edit": "deny", "shell": "deny",
		"external_directory": "deny", "subagent": "deny",
		"execute":                         "allow",
		"mender_task_tools_mender_read":   "allow",
		"mender_task_tools_mender_search": "allow",
		"mender_task_tools_mender_edit":   "allow",
	} {
		if got[action] != expected {
			t.Fatalf("governed OpenCode %s policy changed", action)
		}
	}
	if len(got) != 9 {
		t.Fatal("unexpected extra governed OpenCode permission")
	}
	normal, err := factory.CommandInWorkspace(context.Background(), a, spec.WorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(normal.Env, "OPENCODE_DISABLE_PROJECT_CONFIG=1") {
		t.Fatal("ordinary Agent behavior silently changed")
	}
	var normalPolicy string
	for _, item := range normal.Env {
		if strings.HasPrefix(item, "OPENCODE_CONFIG_CONTENT=") {
			normalPolicy = strings.TrimPrefix(item, "OPENCODE_CONFIG_CONTENT=")
		}
	}
	var normalConfig map[string][]map[string]string
	if err := json.Unmarshal([]byte(normalPolicy), &normalConfig); err != nil {
		t.Fatal(err)
	}
	for _, rule := range normalConfig["permissions"] {
		if rule["action"] == "execute" && rule["effect"] != "deny" {
			t.Fatal("ordinary/Host Agent Code Mode was silently enabled")
		}
	}
}
