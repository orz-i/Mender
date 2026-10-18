package bootstrap

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestResolveMCPToolSourcesUsesVerifiedOwnerAndRequestScope(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "mcp-sources.db"),
		AgentExecutor: fixtureAgentExecutor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	caller := agentOwnerContext(t, r)
	agent, err := r.CreateAgentWithTools(ctx, agentapplication.CreateRequest{
		Name: "Development Agent", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	agentSuite, err := r.AgentTools(ctx, agent.ID(), caller)
	if err != nil || agentSuite.Status != "ready" {
		t.Fatalf("agent suite: %+v %v", agentSuite, err)
	}
	sandbox, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "Rocky Workspace", Root: t.TempDir(), Isolation: "host",
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	sandboxSuite, err := r.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), caller)
	if err != nil || sandboxSuite.Status != "ready" {
		t.Fatalf("sandbox suite: %+v %v", sandboxSuite, err)
	}
	unconfigured, err := r.Catalog.RegisterProvider.Execute(ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "unconfigured", DisplayName: "Potentially misleading",
	})
	if err != nil {
		t.Fatal(err)
	}

	agentID, err := catalogdomain.ParseProviderID(agentSuite.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	sandboxID, err := catalogdomain.ParseProviderID(sandboxSuite.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := r.resolveMCPToolSources(ctx, []catalogdomain.ProviderID{agentID, sandboxID, agentID})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("scope returned %d sources, want only 2", len(sources))
	}
	if got := sources[agentID]; got.Kind != "agent" || got.Name != "Development Agent" ||
		got.ResourceID != string(agent.ID()) {
		t.Fatalf("agent attribution = %+v", got)
	}
	if got := sources[sandboxID]; got.Kind != "sandbox" || got.Name != "Rocky Workspace" ||
		got.ResourceID != sandbox.ID().String() {
		t.Fatalf("sandbox attribution = %+v", got)
	}

	orphan, err := r.resolveMCPToolSources(ctx, []catalogdomain.ProviderID{unconfigured.ID()})
	if err != nil {
		t.Fatal(err)
	}
	if got := orphan[unconfigured.ID()]; got.Kind != "unclaimed" || got.ResourceID != "" ||
		strings.Contains(got.Name, "misleading") {
		t.Fatalf("unverified source should not expose guessed identity: %+v", got)
	}
	empty, err := r.resolveMCPToolSources(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty scope = %+v error %v", empty, err)
	}
}
