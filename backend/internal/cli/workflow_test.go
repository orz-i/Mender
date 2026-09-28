package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
)

func TestCLIAddsModelToolsProviderEngines(t *testing.T) {
	t.Parallel()

	for _, engine := range []string{"openai", "anthropic", "gemini"} {
		engine := engine
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			dbPath := filepath.Join(t.TempDir(), "mender.db")
			registered := executeJSONCommand(
				t,
				dbPath,
				"provider", "add", "model-tools",
				"--namespace", "research-"+engine,
				"--name", "Research "+engine,
				"--engine", engine,
				"--model", engine+"-model",
				"--credential", "credential://model/api",
				"--capability", "web_search",
			)
			provider, ok := registered["provider"].(map[string]any)
			if !ok {
				t.Fatalf("provider add output = %#v", registered)
			}
			if provider["pluginId"] != "io.mender.provider.model-tools" {
				t.Fatalf("provider pluginId = %#v", provider["pluginId"])
			}
			syncResult, ok := registered["sync"].(map[string]any)
			if !ok || syncResult["total"] != float64(1) {
				t.Fatalf("provider sync = %#v", registered["sync"])
			}
		})
	}
}

func TestCLIAddsOpenAPIProvider(t *testing.T) {
	t.Parallel()

	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		spec := `openapi: 3.1.2
info:
  title: CLI OpenAPI
  version: "1"
servers:
  - url: ` + serverURL + `
paths:
  /ping:
    get:
      operationId: ping
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
`
		_, _ = w.Write([]byte(spec))
	})
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	server := httptest.NewServer(mux)
	serverURL = server.URL
	t.Cleanup(server.Close)

	dbPath := filepath.Join(t.TempDir(), "mender.db")
	registered := executeJSONCommand(
		t,
		dbPath,
		"provider", "add", "openapi",
		"--namespace", "openapi-cli",
		"--name", "OpenAPI CLI",
		"--spec-url", server.URL+"/openapi.yaml",
	)
	provider, ok := registered["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider add output = %#v", registered)
	}
	if provider["pluginId"] != "io.mender.provider.openapi" {
		t.Fatalf("provider pluginId = %#v", provider["pluginId"])
	}
	syncResult, ok := registered["sync"].(map[string]any)
	if !ok || syncResult["total"] != float64(1) {
		t.Fatalf("provider sync = %#v", registered["sync"])
	}

	listed := executeJSONCommand(t, dbPath, "provider", "list")
	providers, ok := listed["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("provider list = %#v", listed)
	}
	item, _ := providers[0].(map[string]any)
	if item["pluginId"] != "io.mender.provider.openapi" {
		t.Fatalf("listed provider = %#v", item)
	}
}

func TestCLILocalWorkflowWithPersistedHTTPProvider(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "cli-upstream", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:         "echo",
			Title:        "Echo",
			Description:  "Echo JSON input",
			InputSchema:  json.RawMessage("{\"type\":\"object\"}"),
			OutputSchema: json.RawMessage("{\"type\":\"object\"}"),
		},
		func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			upstreamCalls.Add(1)
			var input map[string]any
			if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{StructuredContent: input}, nil
		},
	)

	httpServer := httptest.NewServer(
		mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return upstream },
			&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
		),
	)
	t.Cleanup(httpServer.Close)

	dbPath := filepath.Join(t.TempDir(), "mender.db")
	execute := func(args ...string) map[string]any {
		t.Helper()
		return executeJSONCommand(t, dbPath, args...)
	}

	registered := execute(
		"provider", "add", "mcp", "http",
		"--namespace", "upstream",
		"--name", "Upstream",
		"--endpoint", httpServer.URL,
	)
	provider, ok := registered["provider"].(map[string]any)
	if !ok || provider["namespace"] != "upstream" {
		t.Fatalf("provider add output = %#v", registered)
	}

	execute("provider", "sync", "upstream")

	search := execute("tool", "search", "echo")
	tools, ok := search["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tool search output = %#v", search)
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tool search item = %#v", tools[0])
	}
	toolID, ok := tool["toolId"].(string)
	if !ok || toolID == "" {
		t.Fatalf("toolId = %#v", tool["toolId"])
	}

	described := execute("tool", "describe", toolID)
	if described["name"] != "echo" {
		t.Fatalf("tool describe output = %#v", described)
	}
	if _, ok := described["inputSchema"].(map[string]any); !ok {
		t.Fatalf("tool describe inputSchema = %#v", described["inputSchema"])
	}

	execute(
		"profile", "create",
		"--slug", "default",
		"--name", "Default",
		"--mode", "hybrid",
	)
	configuredProfile := execute(
		"profile", "add-tool", "default", toolID,
		"--direct",
		"--alias", "echo_cli",
		"--title", "CLI Echo",
		"--description", "Echo configured through the CLI",
		"--policy", "inherit",
		"--concurrency", "serial",
	)
	configuredTools, ok := configuredProfile["tools"].([]any)
	if !ok || len(configuredTools) != 1 {
		t.Fatalf("configured Profile tools = %#v", configuredProfile)
	}
	configuredBinding, _ := configuredTools[0].(map[string]any)
	if configuredBinding["alias"] != "echo_cli" ||
		configuredBinding["titleOverride"] != "CLI Echo" ||
		configuredBinding["descriptionOverride"] != "Echo configured through the CLI" ||
		configuredBinding["policyConstraint"] != "inherit" ||
		configuredBinding["concurrencyConstraint"] != "serial" {
		t.Fatalf("configured Profile binding = %#v", configuredBinding)
	}
	restricted := execute(
		"profile", "create",
		"--slug", "restricted",
		"--name", "Restricted",
		"--mode", "dynamic",
	)
	restrictedID, _ := restricted["profileId"].(string)
	if restrictedID == "" {
		t.Fatalf("restricted profile = %#v", restricted)
	}

	scoped := execute("tool", "search", "echo", "--profile", "default")
	scopedTools, ok := scoped["tools"].([]any)
	if !ok || len(scopedTools) != 1 {
		t.Fatalf("profile-scoped search output = %#v", scoped)
	}

	called := execute(
		"tool", "call", toolID,
		"--args", "{\"value\":\"hello\"}",
	)
	if called["kind"] != "complete" {
		t.Fatalf("tool call output = %#v", called)
	}
	content, ok := called["content"].(map[string]any)
	if !ok || content["value"] != "hello" {
		t.Fatalf("tool call content = %#v", called["content"])
	}
	if invocationID, ok := called["invocationId"].(string); !ok || invocationID == "" {
		t.Fatalf("invocationId = %#v", called["invocationId"])
	}

	profileCalled := execute(
		"tool", "call", toolID,
		"--profile", "default",
		"--args", "{\"value\":\"profile\"}",
	)
	if profileCalled["kind"] != "complete" {
		t.Fatalf("profile tool call output = %#v", profileCalled)
	}

	var rejectedStdout, rejectedStderr bytes.Buffer
	rejectedCode := Run(
		context.Background(),
		[]string{
			"--db", dbPath,
			"--output", "json",
			"tool", "call", toolID,
			"--profile", restrictedID,
			"--args", "{}",
		},
		&rejectedStdout,
		&rejectedStderr,
	)
	if rejectedCode != ExitConfig {
		t.Fatalf(
			"restricted call exit = %d, want %d; stderr=%q stdout=%q",
			rejectedCode,
			ExitConfig,
			rejectedStderr.String(),
			rejectedStdout.String(),
		)
	}
	var rejected map[string]any
	if err := json.Unmarshal(rejectedStdout.Bytes(), &rejected); err != nil {
		t.Fatalf("restricted call JSON error = %v output=%q", err, rejectedStdout.String())
	}
	if rejected["kind"] != "rejected" || rejected["code"] != "tool_outside_scope" {
		t.Fatalf("restricted call outcome = %#v", rejected)
	}

	var servedHTTP atomic.Bool
	runServeCommand(t, dbPath, Options{
		ServeHTTP: func(ctx context.Context, _ string, adapter *mcpserver.Adapter) error {
			servedHTTP.Store(true)
			assertServedProfileTools(t, ctx, adapter, "upstream__echo_cli")
			return nil
		},
	}, "serve", "mcp", "--profile", "default", "--http", "127.0.0.1:0")
	if !servedHTTP.Load() {
		t.Fatal("serve HTTP adapter was not invoked")
	}

	var servedStdio atomic.Bool
	runServeCommand(t, dbPath, Options{
		ServeStdio: func(ctx context.Context, adapter *mcpserver.Adapter) error {
			servedStdio.Store(true)
			assertServedProfileTools(t, ctx, adapter, "upstream__echo_cli")
			return nil
		},
	}, "serve", "mcp", "--profile", "default", "--stdio")
	if !servedStdio.Load() {
		t.Fatal("serve stdio adapter was not invoked")
	}

	if got := upstreamCalls.Load(); got != 2 {
		t.Fatalf("upstream call count = %d, want 2", got)
	}
}

func executeJSONCommand(
	t *testing.T,
	dbPath string,
	args ...string,
) map[string]any {
	t.Helper()

	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Options{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	root.SetArgs(append(
		[]string{"--db", dbPath, "--output", "json"},
		args...,
	))
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf(
			"command %v error = %v stderr=%q stdout=%q",
			args,
			err,
			stderr.String(),
			stdout.String(),
		)
	}

	decoder := json.NewDecoder(&stdout)
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("command %v decode error = %v output=%q", args, err, stdout.String())
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf(
			"command %v must emit exactly one JSON document; trailing decode error=%v output=%q",
			args,
			err,
			stdout.String(),
		)
	}
	return value
}

func TestServeMCPRequiresExactlyOneTransport(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"serve", "mcp", "--profile", "default"},
		{"serve", "mcp", "--profile", "default", "--stdio", "--http", "127.0.0.1:8080"},
	} {
		var stdout, stderr bytes.Buffer
		root := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
		root.SetArgs(append([]string{
			"--db", filepath.Join(t.TempDir(), "mender.db"),
		}, args...))
		if err := root.ExecuteContext(context.Background()); err == nil {
			t.Fatalf("command %v expected validation error", args)
		}
	}
}

func runServeCommand(
	t *testing.T,
	dbPath string,
	options Options,
	args ...string,
) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	options.Stdout = &stdout
	options.Stderr = &stderr
	root := NewRootCommand(options)
	root.SetArgs(append([]string{"--db", dbPath}, args...))
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("serve command %v error = %v stderr=%q", args, err, stderr.String())
	}
}

func assertServedProfileTools(
	t *testing.T,
	ctx context.Context,
	adapter *mcpserver.Adapter,
	expectedDirect string,
) {
	t.Helper()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := adapter.MCPServer().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("served MCP server.Connect() error = %v", err)
	}
	defer func() { _ = serverSession.Close() }()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "cli-serve-test", Version: "1.0.0"},
		nil,
	)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("served MCP client.Connect() error = %v", err)
	}
	defer func() { _ = session.Close() }()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("served MCP ListTools() error = %v", err)
	}
	found := false
	for _, tool := range listed.Tools {
		if tool.Name == expectedDirect {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("served MCP tools do not include %q: %+v", expectedDirect, listed.Tools)
	}
}
