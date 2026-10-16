package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentdelegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// A real MCP client session must be able to dispatch a governed asynchronous
// fixture task and retrieve its result. No CLI, real account, or remote
// endpoint is involved in this test.
func TestAgentDelegationThroughMCPClientSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "mcp-agent.db"),
		AgentExecutor: fixtureAgentExecutor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.Agents.Create(ctx, agentapp.CreateRequest{
		Name: "MCP Fixture", Backend: agentdomain.BackendCodex, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(map[string]string{"agentId": string(agent.ID())})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
		Namespace: "agent-mcp-fixture", DisplayName: "Agent MCP Fixture",
	}, agentdelegation.ID, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.syncConfiguredProviderForOwner(ctx, provider.ID(), owner, true); err != nil {
		t.Fatal(err)
	}
	tools, err := r.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "agent-mcp", DisplayName: "Agent MCP Fixture", Mode: profiledomain.ExposureDirect,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		profile, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
			ProfileSlug: profile.Slug(), ToolID: tool.ID(), Direct: true,
		})
		if err != nil {
			t.Fatal(err)
		}
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
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "agent-delegation-fixture", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, tool := range listed.Tools {
		names[tool.Name] = tool.Name

	}
	delegate := "agent-mcp-fixture__delegate"
	resultName := "agent-mcp-fixture__result"
	if names[delegate] == "" || names[resultName] == "" {
		t.Fatalf("fixture Agent tools missing from real MCP list: %+v", listed.Tools)
	}
	invoke := func(name string, args map[string]string) (*mcp.CallToolResult, error) {
		t.Helper()
		return session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	}
	denied, err := invoke(delegate, map[string]string{"prompt": "before grant"})
	if err != nil {
		t.Fatal(err)
	}
	if !denied.IsError {
		t.Fatal("MCP Profile delegated Agent without AgentGrant")
	}
	if err := r.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	submitted, err := invoke(delegate, map[string]string{"prompt": "MCP delegated fixture"})
	if err != nil || submitted.IsError {
		t.Fatalf("MCP Agent dispatch failed: %v / %+v", err, submitted)
	}
	rawResult, ok := submitted.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("unexpected MCP receipt: %#v", submitted.StructuredContent)
	}
	id, ok := rawResult["taskId"].(string)
	if !ok || id == "" {
		t.Fatalf("MCP TaskID missing: %#v", rawResult)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		observed, err := invoke(resultName, map[string]string{"taskId": id})
		if err != nil || observed.IsError {
			t.Fatalf("MCP result access failed: %v / %+v", err, observed)
		}
		value, ok := observed.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("MCP result not structured: %#v", observed.StructuredContent)
		}
		if value["status"] == "completed" {
			if value["result"] != "fixture: MCP delegated fixture" {
				t.Fatalf("wrong Agent task result: %#v", value)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("MCP Agent task not completed: %#v", value)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := r.Agents.Revoke(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	revoked, err := invoke(delegate, map[string]string{"prompt": "after revoke"})
	if err != nil {
		t.Fatal(err)
	}
	if !revoked.IsError {
		t.Fatal("revoked MCP Profile submitted task")
	}
}
