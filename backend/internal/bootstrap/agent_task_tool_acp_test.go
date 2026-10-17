package bootstrap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	runtimedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

func TestGenericTaskToolACPFixtureProcess(t *testing.T) {
	if os.Getenv("MENDER_TASK_TOOL_ACP_HELPER") != "1" {
		return
	}
	scan := bufio.NewScanner(os.Stdin)
	write := bufio.NewWriter(os.Stdout)
	send := func(m any) {
		value, _ := json.Marshal(m)
		_, _ = fmt.Fprintln(write, string(value))
		_ = write.Flush()
	}
	var session *mcp.ClientSession
	for scan.Scan() {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scan.Bytes(), &msg) != nil {
			os.Exit(190)
		}
		switch msg.Method {
		case "initialize":
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{
				"protocolVersion":   1,
				"agentCapabilities": map[string]any{"mcpCapabilities": map[string]bool{"http": true}},
			}})
		case "session/new":
			var req struct {
				MCPServers []struct {
					Type    string `json:"type"`
					Name    string `json:"name"`
					URL     string `json:"url"`
					Headers []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"headers"`
				} `json:"mcpServers"`
			}
			if json.Unmarshal(msg.Params, &req) != nil || len(req.MCPServers) != 1 {
				os.Exit(191)
			}
			server := req.MCPServers[0]
			if server.Type != "http" || server.Name != "mender-task-tools" || len(server.Headers) != 1 {
				os.Exit(192)
			}
			client := mcp.NewClient(&mcp.Implementation{Name: "generic-synthetic-agent", Version: "1.0.0"}, nil)
			transport := &mcp.StreamableClientTransport{
				Endpoint: server.URL, DisableStandaloneSSE: true,
				HTTPClient: bearerTestHTTPClient(strings.TrimPrefix(server.Headers[0].Value, "Bearer ")),
			}
			var err error
			session, err = client.Connect(context.Background(), transport, nil)
			if err != nil {
				os.Exit(193)
			}
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"sessionId": "fixture-generic-session"}})
		case "session/prompt":
			if session == nil {
				os.Exit(194)
			}
			confined := os.Getenv("MENDER_TASK_TOOL_ACP_CONFINED") == "1"
			tools, err := session.ListTools(context.Background(), nil)
			expected := 1
			if confined {
				expected = 2
			}
			if err != nil || len(tools.Tools) != expected {
				os.Exit(195)
			}
			visible := make(map[string]bool, len(tools.Tools))
			for _, tool := range tools.Tools {
				visible[tool.Name] = true
			}
			if len(visible) != expected || !visible["mender_read"] {
				os.Exit(195)
			}
			if confined {
				if !visible["mender_edit"] {
					os.Exit(199)
				}
				// The process is in a Seatbelt read-only root. A built-in/native
				// Agent file write must fail without a Mender Tool invocation.
				if err := os.WriteFile("native-bypass.txt", []byte("must not exist"), 0o600); err == nil {
					os.Exit(200)
				}
				// Only the *existing* canonical Mender workspace.edit can create
				// this disposable file with current Profile and Sandbox grants.
				edited, e := session.CallTool(context.Background(), &mcp.CallToolParams{
					Name:      "mender_edit",
					Arguments: json.RawMessage(`{"path":"controlled-edit.txt","operation":"create","content":"through Mender only"}`),
				})
				if e != nil || edited.IsError {
					os.Exit(201)
				}
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "mender_read", Arguments: json.RawMessage(`{"path":"review.txt"}`),
			})
			if err != nil || result.IsError {
				os.Exit(196)
			}
			payload, _ := json.Marshal(result.StructuredContent)
			if !strings.Contains(string(payload), "governed temporary contents") {
				os.Exit(197)
			}
			send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": "fixture-generic-session", "update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]string{"type": "text", "text": "governed temporary contents"},
				},
			}})
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"stopReason": "end_turn"}})
			_ = session.Close()
		default:
			os.Exit(198)
		}
	}
	os.Exit(0)
}
func TestGenericACPTaskToolsUseExistingMenderGovernedInvocation(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	server, err := f.runtime.openAgentTaskToolMCPServer(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.read"]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.close() }()
	runner := agentacp.Runner{NewCommandWithConfig: func(ctx context.Context, _ agentdomain.Agent, _ agentapp.RunConfig) (*exec.Cmd, error) {
		executable, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestGenericTaskToolACPFixtureProcess$") // #nosec G204 -- test-only trusted current Go test executable and fixed args.
		cmd.Env = append(os.Environ(), "MENDER_TASK_TOOL_ACP_HELPER=1")
		return cmd, nil
	}, NewCommand: func(context.Context, agentdomain.Agent) (*exec.Cmd, error) {
		return nil, errors.New("unsafe legacy launcher invoked")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	result, err := runner.ExecuteConfigured(ctx, f.agent, "read only governed Tool in fixture",
		agentapp.RunConfig{WorkspaceRoot: f.root, ToolSupply: agentapp.ToolSupply{
			Delivery: agentapp.ToolDeliveryMCPHTTP, Endpoint: server.endpoint, BearerToken: server.token,
		}})
	if err != nil || result != "governed temporary contents" {
		t.Fatalf("generic Task-scoped MCP supply failed: %q %v", result, err)
	}
	// No extra Tool IDs were introduced to the Catalog, and the fixture was an
	// existing Grok identity: MCP capability, not Agent brand, enabled supply.
	counts, err := f.runtime.Catalog.store.ListProviderTools(ctx, func() catalogdomain.ProviderID {
		tools, err := f.runtime.SandboxWorkspaceTools(ctx, f.sandbox.ID(), f.owner)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := catalogdomain.ParseProviderID(tools.ProviderID)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}())
	if err != nil || len(counts) != 4 {
		t.Fatalf("canonical Tool inventory changed: %d %v", len(counts), err)
	}
}

func bearerTestHTTPClient(token string) *http.Client {
	return &http.Client{Transport: taskBearerTransport{base: http.DefaultTransport, token: token}}
}

func TestRestrictedGenericACPAgentCannotNativeWriteButCanInvokeMenderEdit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native macOS Seatbelt acceptance")
	}
	isolate := processisolation.New()
	if !isolate.Info().RestrictedHostAvailable {
		t.Skip("restricted-host process unavailable")
	}
	f := setupGovernedTaskToolFixture(t)
	server, err := f.runtime.openAgentTaskToolMCPServer(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.read"], f.ids["workspace.edit"]})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.close() }()
	runner := agentacp.Runner{
		NewCommandWithConfig: func(ctx context.Context, _ agentdomain.Agent, _ agentapp.RunConfig) (*exec.Cmd, error) {
			current, err := os.Executable()
			if err != nil {
				return nil, err
			}
			return isolate.Command(ctx, runtimedomain.IsolationRestrictedHost, processisolation.CommandSpec{
				Executable:       current,
				Arguments:        []string{"-test.run=^TestGenericTaskToolACPFixtureProcess$"},
				WorkingDirectory: f.root,
				PermittedRoot:    f.root,
				RootWritable:     false,
				Environment: []string{
					"PATH=/usr/bin:/bin:/opt/homebrew/bin",
					"TMPDIR=" + os.TempDir(),
					"MENDER_TASK_TOOL_ACP_HELPER=1",
					"MENDER_TASK_TOOL_ACP_CONFINED=1",
				},
			})
		},
		NewCommand: func(context.Context, agentdomain.Agent) (*exec.Cmd, error) {
			return nil, errors.New("unrestricted Agent process fallback must not run")
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
	defer cancel()
	result, err := runner.ExecuteConfigured(ctx, f.agent, "synthetic confined Agent tool call",
		agentapp.RunConfig{WorkspaceRoot: f.root, ToolSupply: agentapp.ToolSupply{
			Delivery: agentapp.ToolDeliveryMCPHTTP, Endpoint: server.endpoint, BearerToken: server.token,
		}})
	if err != nil || result != "governed temporary contents" {
		t.Fatalf("confined ACP Tool supply did not complete: result=%q err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "native-bypass.txt")); !os.IsNotExist(err) {
		t.Fatalf("restricted native Agent wrote directly: %v", err)
	}
	created, err := os.ReadFile(filepath.Join(f.root, "controlled-edit.txt"))
	if err != nil || string(created) != "through Mender only" {
		t.Fatalf("Mender-governed edit did not execute: %q err=%v", created, err)
	}
}
