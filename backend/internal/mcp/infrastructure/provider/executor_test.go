package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestConnectionExecutesMCPTool(t *testing.T) {
	t.Parallel()

	connection := connectTestServer(t, func(server *mcp.Server) {
		server.AddTool(
			&mcp.Tool{
				Name:         "echo",
				InputSchema:  json.RawMessage(`{"type":"object"}`),
				OutputSchema: json.RawMessage(`{"type":"object"}`),
			},
			func(
				_ context.Context,
				request *mcp.CallToolRequest,
			) (*mcp.CallToolResult, error) {
				var input map[string]any
				if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
					return nil, err
				}
				return &mcp.CallToolResult{
					StructuredContent: map[string]any{
						"echo": input["value"],
					},
				}, nil
			},
		)
	})

	arguments, _ := invocationdomain.NewJSONValue([]byte(`{"value":"hello"}`))
	result, err := connection.Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      executorTestTool(t, "echo"),
			Arguments: arguments,
		},
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
	if output["echo"] != "hello" {
		t.Fatalf("output = %#v", output)
	}
}

func TestConnectionMapsMCPErrorResultToExecutionError(t *testing.T) {
	t.Parallel()

	connection := connectTestServer(t, func(server *mcp.Server) {
		server.AddTool(
			&mcp.Tool{Name: "fail", InputSchema: json.RawMessage(`{"type":"object"}`)},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{Text: "provider rejected request"},
					},
				}, nil
			},
		)
	})

	arguments, _ := invocationdomain.NewJSONValue([]byte(`{}`))
	_, err := connection.Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      executorTestTool(t, "fail"),
			Arguments: arguments,
		},
		nil,
	)
	if !errors.Is(err, ErrToolResult) {
		t.Fatalf("Execute() error = %v, want ErrToolResult", err)
	}
	var resultErr *ToolResultError
	if !errors.As(err, &resultErr) {
		t.Fatalf("Execute() error = %T, want ToolResultError", err)
	}
	if resultErr.DiagnosticSummary() != "provider rejected request" {
		t.Fatalf("DiagnosticSummary() = %q", resultErr.DiagnosticSummary())
	}
	if err.Error() != ErrToolResult.Error() {
		t.Fatalf("Error() = %q, want generic error", err.Error())
	}
}

func TestConnectionMapsInputRequiredWithoutAutoRetry(t *testing.T) {
	t.Parallel()

	connection := connectTestServer(t, func(server *mcp.Server) {
		server.AddTool(
			&mcp.Tool{Name: "confirm", InputSchema: json.RawMessage(`{"type":"object"}`)},
			func(
				_ context.Context,
				request *mcp.CallToolRequest,
			) (*mcp.CallToolResult, error) {
				if len(request.Params.InputResponses) != 0 {
					return &mcp.CallToolResult{
						StructuredContent: map[string]any{"confirmed": true},
					}, nil
				}
				return &mcp.CallToolResult{
					InputRequests: mcp.InputRequestMap{
						"confirm": &mcp.ElicitParams{
							Message: "Confirm operation",
							RequestedSchema: map[string]any{
								"type": "object",
								"properties": map[string]any{
									"ok": map[string]any{"type": "boolean"},
								},
							},
						},
					},
					RequestState: "opaque-state",
				}, nil
			},
		)
	})

	arguments, _ := invocationdomain.NewJSONValue([]byte(`{}`))
	result, err := connection.Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      executorTestTool(t, "confirm"),
			Arguments: arguments,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Kind() != invocationdomain.ResultKindInputRequired {
		t.Fatalf("Result kind = %q, want input_required", result.Kind())
	}
	if result.RequestID() == "" {
		t.Fatal("InputRequired RequestID is empty")
	}
}

func TestConnectionRejectsNonObjectArguments(t *testing.T) {
	t.Parallel()

	arguments, _ := invocationdomain.NewJSONValue([]byte(`["not","object"]`))
	_, err := (&Connection{}).Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      executorTestTool(t, "echo"),
			Arguments: arguments,
		},
		nil,
	)
	if !errors.Is(err, ErrArgumentsNotObject) {
		t.Fatalf("Execute() error = %v, want ErrArgumentsNotObject", err)
	}
}

func connectTestServer(t *testing.T, configure func(*mcp.Server)) *Connection {
	t.Helper()

	ctx := context.Background()
	server := mcp.NewServer(
		&mcp.Implementation{Name: "upstream-executor-test", Version: "1.0.0"},
		nil,
	)
	configure(server)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	connection, err := ConnectTransport(ctx, clientTransport)
	if err != nil {
		t.Fatalf("ConnectTransport() error = %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func executorTestTool(t *testing.T, sourceKey string) catalogdomain.Tool {
	t.Helper()

	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	schema, _ := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   sourceKey,
			Name:        sourceKey,
			InputSchema: schema,
		},
		time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}
