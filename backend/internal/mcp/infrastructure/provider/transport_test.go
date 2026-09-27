package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const helperEnv = "MENDER_MCP_STDIO_HELPER"

func TestConnectHTTPDiscoversAndCallsRemoteServer(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "http-upstream-test", Version: "1.0.0"},
		nil,
	)
	addEchoTool(server)

	httpServer := httptest.NewServer(
		mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return server },
			&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
		),
	)
	t.Cleanup(httpServer.Close)

	connection, err := ConnectHTTP(context.Background(), HTTPConfig{
		Endpoint:   httpServer.URL,
		HTTPClient: httpServer.Client(),
		MaxRetries: -1,
	})
	if err != nil {
		t.Fatalf("ConnectHTTP() error = %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	assertConnectionEcho(t, connection)
}

func TestConnectHTTPSendsConfiguredHeaders(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "http-header-upstream-test", Version: "1.0.0"},
		nil,
	)
	addEchoTool(server)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("X-Mender-Test"); got != "header-value" {
			http.Error(w, "missing test header", http.StatusBadRequest)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	connection, err := ConnectHTTP(context.Background(), HTTPConfig{
		Endpoint: httpServer.URL,
		Headers: map[string]string{
			"Authorization": "Bearer test-token",
			"X-Mender-Test": "header-value",
		},
		HTTPClient: httpServer.Client(),
		MaxRetries: -1,
	})
	if err != nil {
		t.Fatalf("ConnectHTTP() error = %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	assertConnectionEcho(t, connection)
}

func TestHTTPHeadersAreNotForwardedAcrossOrigins(t *testing.T) {
	t.Parallel()

	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)

	client, err := httpClientWithHeaders(
		source.Client(),
		source.URL,
		map[string]string{"Authorization": "Bearer test-token"},
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(source.URL)
	if response != nil {
		defer func() { _ = response.Body.Close() }()
	}
	if err == nil || !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("GET redirect error = %v, want cross-origin rejection", err)
	}
	if leaked.Load() {
		t.Fatal("Authorization header leaked to redirect target")
	}
}

func TestConnectCommandDiscoversAndCallsStdioServer(t *testing.T) {
	if os.Getenv(helperEnv) == "1" {
		runStdioHelper()
		return
	}

	connection, err := ConnectCommand(context.Background(), CommandConfig{
		Executable: os.Args[0],
		Args:       []string{"-test.run=^TestConnectCommandDiscoversAndCallsStdioServer$"},
		Environment: map[string]string{
			helperEnv: "1",
		},
	})
	if err != nil {
		t.Fatalf("ConnectCommand() error = %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	assertConnectionEcho(t, connection)
}

func runStdioHelper() {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "stdio-upstream-test", Version: "1.0.0"},
		nil,
	)
	addEchoTool(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func addEchoTool(server *mcp.Server) {
	server.AddTool(
		&mcp.Tool{
			Name:         "echo",
			Title:        "Echo",
			Description:  "Echo input",
			InputSchema:  json.RawMessage(`{"type":"object"}`),
			OutputSchema: json.RawMessage(`{"type":"object"}`),
		},
		func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var input map[string]any
			if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				StructuredContent: input,
			}, nil
		},
	)
}

func assertConnectionEcho(t *testing.T, connection *Connection) {
	t.Helper()

	tools, err := connection.DiscoverTools(context.Background(), testProvider(t))
	if err != nil {
		t.Fatalf("DiscoverTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].SourceKey != "echo" {
		t.Fatalf("DiscoverTools() = %+v", tools)
	}

	arguments, err := invocationdomain.NewJSONValue([]byte(`{"value":"transport"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := connection.Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      executorTestTool(t, "echo"),
			Arguments: arguments,
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Kind() != invocationdomain.ResultKindComplete {
		t.Fatalf("Result kind = %q", result.Kind())
	}

	var output map[string]any
	if err := json.Unmarshal(result.Content().Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output["value"] != "transport" {
		t.Fatalf("output = %#v", output)
	}
}
