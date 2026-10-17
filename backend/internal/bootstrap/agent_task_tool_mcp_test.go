package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

type taskBearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t taskBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	req := request.Clone(request.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}
func connectTaskScopedMCP(t *testing.T, url, token string) (*mcp.ClientSession, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "governed-task-test", Version: "1.0.0"}, nil)
	return client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: url, DisableStandaloneSSE: true,
		HTTPClient: &http.Client{Transport: taskBearerTransport{base: http.DefaultTransport, token: token}},
	}, nil)
}

func TestTaskScopedMCPReusesGovernedWorkspaceToolsNoRecursiveDelegation(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	requested := []catalogdomain.ToolID{f.ids["workspace.read"], f.ids["workspace.edit"]}
	server, err := f.runtime.openAgentTaskToolMCPServer(context.Background(),
		f.task.ID, f.sandbox.ID(), requested)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.close() }()
	session, err := connectTaskScopedMCP(t, server.endpoint, server.token)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	if len(names) != 2 || !names["mender_read"] || !names["mender_edit"] {
		t.Fatalf("unexpected governed Task projection: %+v", names)
	}
	for _, forbidden := range []string{"mender_shell", "agent.delegate", "agent.cancel", "approve"} {
		if names[forbidden] {
			t.Fatalf("unapproved control leaked into Task MCP: %s", forbidden)
		}
	}
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mender_read", Arguments: json.RawMessage(`{"path":"review.txt"}`),
	})
	if err != nil || res.IsError {
		t.Fatalf("Task MCP read rejected: %+v %v", res, err)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(raw), "governed temporary contents") {
		t.Fatalf("canonical read content missing: %s", raw)
	}
	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mender_edit", Arguments: json.RawMessage(`{"path":"mcp-created.txt","operation":"create","content":"MCP governed edit\n"}`),
	})
	if err != nil || res.IsError {
		t.Fatalf("Task MCP edit rejected: %+v %v", res, err)
	}
	after, err := os.ReadFile(filepath.Join(f.root, "mcp-created.txt"))
	if err != nil || string(after) != "MCP governed edit\n" {
		t.Fatalf("MCP route missed temporary file: %q %v", after, err)
	}
	if _, err := connectTaskScopedMCP(t, server.endpoint, "invalid-wrong-token"); err == nil {
		t.Fatal("Client connected to Task MCP without correct token")
	}
	if err := server.close(); err != nil && !strings.Contains(err.Error(), "closed") {
		t.Fatalf("close Task MCP: %v", err)
	}
	if _, err := connectTaskScopedMCP(t, server.endpoint, server.token); err == nil {
		t.Fatal("A new client reused a closed Task tool server")
	}
}
func TestTaskScopedMCPRejectedBindingAndRevokedAgentGrant(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	server, err := f.runtime.openAgentTaskToolMCPServer(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.read"]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.close() }()
	session, err := connectTaskScopedMCP(t, server.endpoint, server.token)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if err := f.runtime.Agents.Revoke(context.Background(), f.agent.ID(), f.profile.ID().String(), f.owner); err != nil {
		t.Fatal(err)
	}
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mender_read", Arguments: json.RawMessage(`{"path":"review.txt"}`),
	})
	if err == nil && res != nil && !res.IsError {
		t.Fatal("MCP Task gateway granted a Tool after AgentGrant revocation")
	}
}

func TestTaskScopedMCPClosesAfterAgentTaskTerminates(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := f.runtime.openAgentTaskToolMCPServer(ctx, f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.read"]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.close() }()
	session, err := connectTaskScopedMCP(t, server.endpoint, server.token)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if err := f.runtime.Agents.Cancel(context.Background(), f.task.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	// Authorization is refused synchronously, before the one-second listener
	// cleanup: an already connected MCP client does not inherit Task privileges.
	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mender_read", Arguments: json.RawMessage(`{"path":"review.txt"}`),
	})
	if callErr == nil && result != nil && !result.IsError {
		t.Fatal("canceled Task served a scoped MCP read")
	}
	limit := time.After(4 * time.Second)
	for {
		_, err := connectTaskScopedMCP(t, server.endpoint, server.token)
		if err != nil {
			break
		}
		select {
		case <-limit:
			t.Fatal("MCP listener survived canceled Task")
		case <-time.After(120 * time.Millisecond):
		}
	}
}
func TestTaskScopedMCPRuntimeShutdownClosesEveryListener(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	first, err := f.runtime.openAgentTaskToolMCPServer(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.read"]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.close() }()
	second, err := f.runtime.openAgentTaskToolMCPServer(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.edit"]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.close() }()
	f.runtime.closeAgentTaskToolMCPServers()
	for _, item := range []*taskScopedMCPServer{first, second} {
		if _, err := connectTaskScopedMCP(t, item.endpoint, item.token); err == nil {
			t.Fatal("Runtime shutdown left Agent Tool MCP listener open")
		}
	}
	if _, err := f.runtime.openAgentTaskToolMCPServer(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.read"]}); err == nil {
		t.Fatal("Runtime shutdown minted a new Task capability")
	}
}

func TestTaskScopedMCPTokensCannotCrossServerInstances(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	first, err := f.runtime.openAgentTaskToolMCPServer(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.read"]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.close() }()
	second, err := f.runtime.openAgentTaskToolMCPServer(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.edit"]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.close() }()
	if _, err := connectTaskScopedMCP(t, first.endpoint, second.token); err == nil {
		t.Fatal("other Task gateway token reused across listeners")
	}
	if _, err := connectTaskScopedMCP(t, second.endpoint, first.token); err == nil {
		t.Fatal("first bearer reused for second Task gateway")
	}
	if session, err := connectTaskScopedMCP(t, first.endpoint, first.token); err != nil {
		t.Fatal(err)
	} else {
		_ = session.Close()
	}
}
