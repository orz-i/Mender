package bootstrap

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
)

// A real MCP client session must be able to dispatch a governed asynchronous
// fixture task and retrieve its result. No CLI, real account, or remote
// endpoint is involved in this test.
func TestAgentDelegationThroughMCPClientSession(t *testing.T) {
	ctx := t.Context()
	finished := make(chan struct{})
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "mcp-agent.db"),
		AgentExecutor: fixtureAgentExecutor{finished: finished},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "MCP Fixture", Backend: agentdomain.BackendCodex, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	ids := agentOwnedToolIDs(t, r, ctx, agent.ID(), owner)
	profile, err := createMCPProfileFixture(t, r, ctx, profileapp.CreateProfileRequest{
		Slug: "agent-mcp", DisplayName: "Agent MCP Fixture",
	}, "direct")

	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		profile, err = addMCPProfileToolFixture(t, r, ctx, profileapp.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: id}, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Migration and resource setup are bounded by the Go test deadline, not
	// the protocol exercise budget. Start that budget once the fixture exists.
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	preGrant, err := r.typedSupply.Project(ctx, profile.ID().String())
	if err != nil || len(preGrant.Sources) != 0 {
		t.Fatalf("Agent Tool Source leaked before grant: %+v %v", preGrant, err)
	}
	if err := r.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
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
	delegate, resultName, resumeName := "", "", ""
	for _, item := range listed.Tools {
		switch {
		case strings.HasSuffix(item.Name, "__delegate"):
			delegate = item.Name
		case strings.HasSuffix(item.Name, "__result"):
			resultName = item.Name
		case strings.HasSuffix(item.Name, "__resume"):
			resumeName = item.Name
		}
	}
	if delegate == "" || resultName == "" || resumeName == "" {
		t.Fatalf("Agent owned MCP tools missing: %+v", listed.Tools)
	}
	invoke := func(name string, args map[string]string) (*mcp.CallToolResult, error) {
		t.Helper()
		return session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
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
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatalf("fixture did not finish native terminal evidence: %v", ctx.Err())
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
	// The advertised but unsupported restricted Host continuation must be
	// a precise, audited MCP Tool error; it must not create a replacement Task.
	beforeTasks, err := r.Agents.Tasks(ctx, agent.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	resumeErr, err := invoke(resumeName, map[string]string{
		"taskId": id, "prompt": "must not be replayed",
	})
	if err != nil || resumeErr == nil || !resumeErr.IsError {
		t.Fatalf("MCP unsupported session resume was not a Tool error: result=%+v err=%v", resumeErr, err)
	}
	failure, ok := resumeErr.StructuredContent.(map[string]any)
	if !ok || failure["kind"] != "failed" || failure["source"] != "tool" ||
		failure["code"] != "tool_error" || failure["message"] != "Session continuation is unavailable for this task or Agent. No new task was created; do not replay the previous prompt." {
		t.Fatalf("unsafe MCP continuation failure: %#v", resumeErr.StructuredContent)
	}
	afterTasks, err := r.Agents.Tasks(ctx, agent.ID(), owner)
	if err != nil || len(afterTasks) != len(beforeTasks) {
		t.Fatalf("unsupported resume created a Task: before=%d after=%d err=%v", len(beforeTasks), len(afterTasks), err)
	}
	if err := r.Agents.Revoke(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	revoked, err := invoke(delegate, map[string]string{"prompt": "after revoke"})
	if err == nil && (revoked == nil || !revoked.IsError) {
		t.Fatal("revoked MCP Profile submitted task")
	}
}
