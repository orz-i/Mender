package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
)

// This is an end-to-end MCP transport regression with two distinct Agent
// instances sharing the exact same backend/tool names and no real agent calls.
func TestMCPDiscoveryUsesRealAgentOwnerNotFacadeDisplay(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "source-discovery.db"),
		AgentExecutor: fixtureAgentExecutor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	caller := agentOwnerContext(t, r)

	type selectedAgent struct {
		ref     catalogdomain.SourceRef
		agentID agentdomain.ID
		name    string
		toolID  catalogdomain.ToolID
	}
	agents := make([]selectedAgent, 0, 2)
	for _, agentName := range []string{"QA OpenCode", "QA Antigravity"} {
		agent, err := r.CreateAgentWithTools(ctx, agentapplication.CreateRequest{
			Name: agentName, Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
		}, caller)
		if err != nil {
			t.Fatal(err)
		}
		suite, err := r.AgentTools(ctx, agent.ID(), caller)
		if err != nil || suite.Status != "ready" {
			t.Fatalf("agent tools: %+v %v", suite, err)
		}
		ref, err := catalogdomain.AgentSource(string(agent.ID()))
		if err != nil {
			t.Fatal(err)
		}
		var delegate catalogdomain.ToolID
		for _, tool := range suite.Tools {
			if tool.SourceKey == "agent.delegate" {
				delegate, err = catalogdomain.ParseToolID(tool.ToolID)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		if delegate == "" {
			t.Fatalf("missing Agent-owned delegate: %s", agentName)
		}
		agents = append(agents, selectedAgent{ref: ref, agentID: agent.ID(), name: agentName, toolID: delegate})
	}

	profile, err := createMCPProfileFixture(t, r, ctx, profileapplication.CreateProfileRequest{
		Slug: "agent-owner-search", DisplayName: "Agent owner search",
	}, "dynamic")

	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range agents {
		profile, err = r.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{
			ProfileSlug: profile.Slug(), ToolID: agent.toolID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Agents.Grant(ctx, agent.agentID, profile.ID().String(), caller); err != nil {
			t.Fatal(err)
		}
	}

	profile, err = r.Profiles.Get.Execute(ctx, profile.Slug())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := r.NewMCPServer(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := adapter.MCPServer().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "source-discovery-test", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	search := func(arguments map[string]any) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "mender_search_tools", Arguments: arguments,
		})
		if err != nil || result.IsError {
			t.Fatalf("MCP source search: %v / %+v", err, result)
		}
		output, ok := result.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("unexpected structured result %T", result.StructuredContent)
		}
		return output
	}

	found := search(map[string]any{"query": "agent", "view": "sources"})
	sources, ok := found["sources"].([]any)
	if !ok || len(sources) != 2 {
		t.Fatalf("search agent sources = %#v", found)
	}
	for _, agent := range agents {
		exact := search(map[string]any{"query": agent.name, "view": "sources"})
		sources, ok := exact["sources"].([]any)
		if !ok || len(sources) != 1 {
			t.Fatalf("source name %s = %#v", agent.name, exact)
		}
		source := sources[0].(map[string]any)
		if source["sourceId"] != agent.ref.String() ||
			source["sourceName"] != agent.name || source["sourceKind"] != "agent" ||
			source["toolCount"] != float64(1) && source["toolCount"] != 1 {
			t.Fatalf("source mismatch: %s => %#v", agent.name, source)
		}
		toolsResult := search(map[string]any{"sourceId": agent.ref.String(), "view": "tools"})
		tools, ok := toolsResult["tools"].([]any)
		if !ok || len(tools) != 1 || tools[0].(map[string]any)["toolId"] != agent.toolID.String() {
			t.Fatalf("source %s tools = %#v", agent.name, toolsResult)
		}
	}
	// Standard MCP describe remains identity-locked: the Profile sees only
	// the two delegated tool IDs, never sibling status/result/cancel Tools.
	store, ok := r.Catalog.store.(interface {
		ListSourceTools(context.Context, catalogdomain.SourceRef) ([]catalogdomain.Tool, error)
	})
	if !ok {
		t.Fatal("typed Catalog lookup unavailable")
	}
	tools, err := store.ListSourceTools(ctx, agents[0].ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.ID() != agents[0].toolID {
			raw, _ := json.Marshal(map[string]string{"toolId": tool.ID().String()})
			if _, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "mender_describe_tool", Arguments: raw,
			}); err == nil {
				t.Fatalf("describe leaked hidden ToolID %s", tool.ID())
			}
			break
		}
	}
}
