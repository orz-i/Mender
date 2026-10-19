package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestOpenCodeBuiltinMarketplaceToGovernedAgentCreation(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "opencode-marketplace.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	base := server.URL + "/api/v1"
	catalogue := requestJSON(t, server.Client(), http.MethodGet, base+"/plugins", nil, http.StatusOK)
	found := false
	for _, value := range catalogue["plugins"].([]any) {
		p := value.(map[string]any)
		if p["pluginId"] != "io.mender.agent.opencode" {
			continue
		}
		found = true
		if p["marketplaceAction"] != "create.agent" || p["kind"] != "agent.backend" ||
			p["sourceKind"] != "builtin" || p["extensionId"] != "main" {
			t.Fatalf("OpenCode hidden or offered as wrong extension: %+v", p)
		}
	}
	if !found {
		t.Fatal("OpenCode missing from marketplace")
	}
	agent := requestJSON(t, server.Client(), http.MethodPost, base+"/agents", map[string]any{
		"name": "OpenCode Local", "backend": "opencode", "workspaceRoot": t.TempDir(),
		"isolation": "restricted_host", "defaultModel": "", "defaultReasoningEffort": "",
	}, http.StatusCreated)
	id := agent["agentId"].(string)
	if agent["backend"] != "opencode" || agent["pluginId"] != "io.mender.agent.opencode" ||
		agent["defaultReasoningEffort"] != "" {
		t.Fatalf("incorrect OpenCode resource projection: %+v", agent)
	}
	toolsPath := base + "/agents/" + id + "/tools"
	first := requestJSON(t, server.Client(), http.MethodGet, toolsPath, nil, http.StatusOK)
	if first["status"] != "ready" || len(first["tools"].([]any)) != 5 {
		t.Fatalf("OpenCode missing governed Agent Delegation Tools: %+v", first)
	}
	second := requestJSON(t, server.Client(), http.MethodPost, toolsPath, nil, http.StatusOK)
	if second["providerId"] != first["providerId"] {
		t.Fatal("Opencode changed managed Tool source")
	}
	for i, item := range first["tools"].([]any) {
		if item.(map[string]any)["toolId"] != second["tools"].([]any)[i].(map[string]any)["toolId"] {
			t.Fatal("ToolID changed during OpenCode refresh")
		}
	}
	granted := requestJSON(t, server.Client(), http.MethodGet, base+"/agents/"+id+"/profile-grants", nil, http.StatusOK)
	if len(granted["profileIds"].([]any)) != 0 {
		t.Fatal("new Agent implicitly granted Profiles")
	}
	tasks := requestJSON(t, server.Client(), http.MethodGet, base+"/agents/"+id+"/tasks", nil, http.StatusOK)
	if len(tasks["tasks"].([]any)) != 0 {
		t.Fatal("Agent creation launched a paid task")
	}
}
