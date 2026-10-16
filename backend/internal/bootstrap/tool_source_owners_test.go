package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestToolSourceOwnerProjectionUsesExactResourceBindings(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "sources.db"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	caller := agentOwnerContext(t, r)
	agent, err := r.CreateAgentWithTools(ctx, agentapplication.CreateRequest{
		Name: "QA Agent", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	agentSuite, err := r.AgentTools(ctx, agent.ID(), caller)
	if err != nil || agentSuite.Status != "ready" {
		t.Fatalf("Agent tools: %+v %v", agentSuite, err)
	}
	sandbox, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "QA Workspace", Root: t.TempDir(), Isolation: "host",
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	sandboxSuite, err := r.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), caller)
	if err != nil || sandboxSuite.Status != "ready" {
		t.Fatalf("Sandbox tools: %+v %v", sandboxSuite, err)
	}
	owners, err := r.ListToolSourceOwners(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id      string
		kind    string
		ownerID string
		name    string
	}{
		{agentSuite.ProviderID, "agent", string(agent.ID()), "QA Agent"},
		{sandboxSuite.ProviderID, "sandbox", sandbox.ID().String(), "QA Workspace"},
	} {
		id, err := catalogdomain.ParseProviderID(tc.id)
		if err != nil {
			t.Fatal(err)
		}
		provider, err := r.Catalog.GetProvider.Execute(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		config, err := r.ProviderConfig(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		owner := ToolSourceForProvider(provider, &config, owners)
		if owner.Kind != tc.kind || owner.ResourceID != tc.ownerID || owner.Name != tc.name {
			t.Fatalf("source for %s = %+v, want %s %s %s", id, owner, tc.kind, tc.ownerID, tc.name)
		}
	}
	standalone, err := r.Catalog.RegisterProvider.Execute(ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "independent", DisplayName: "Independent MCP",
	})
	if err != nil {
		t.Fatal(err)
	}
	config, err := providerplugin.NewConfiguration(standalone.ID(), "io.mender.provider.mcp", 1, json.RawMessage("{}"))
	if err != nil {
		t.Fatal(err)
	}
	direct := ToolSourceForProvider(standalone, &config, owners)
	if direct.Kind != "provider" || direct.ResourceID != standalone.ID().String() || direct.Name != "Independent MCP" {
		t.Fatalf("direct source = %+v", direct)
	}
	unmanaged := ToolSourceForProvider(standalone, nil, owners)
	if unmanaged.Kind != "unclaimed" {
		t.Fatalf("unconfigured source = %+v", unmanaged)
	}
	private, err := providerplugin.NewConfiguration(standalone.ID(), "io.mender.provider.workspace", 2, json.RawMessage("{}"))
	if err != nil {
		t.Fatal(err)
	}
	unclaimed := ToolSourceForProvider(standalone, &private, owners)
	if unclaimed.Kind != "unclaimed" || unclaimed.ResourceID != "" {
		t.Fatalf("guessed owner for unclaimed internal facade = %+v", unclaimed)
	}
}
