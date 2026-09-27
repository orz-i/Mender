package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
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

func TestConnectionAuthorizesRequiredCredentialBeforeUpstreamCall(t *testing.T) {
	t.Parallel()

	var upstreamCalls int
	connection := connectTestServer(t, func(server *mcp.Server) {
		server.AddTool(
			&mcp.Tool{
				Name:        "secured",
				InputSchema: json.RawMessage("{\"type\":\"object\"}"),
			},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				upstreamCalls++
				return &mcp.CallToolResult{
					StructuredContent: map[string]any{"ok": true},
				}, nil
			},
		)
	})
	ref, err := credentialdomain.NewReference("github", "work")
	if err != nil {
		t.Fatal(err)
	}
	connection.setRequiredCredentials([]credentialdomain.Reference{ref})
	arguments, _ := invocationdomain.NewJSONValue([]byte("{}"))

	denied := &executorCredentialResolver{err: errors.New("denied")}
	_, err = connection.Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      executorTestTool(t, "secured"),
			Arguments: arguments,
		},
		denied,
		nil,
	)
	if err == nil {
		t.Fatal("Execute() expected Credential authorization error")
	}
	if denied.calls != 1 {
		t.Fatalf("denied resolver calls = %d, want 1", denied.calls)
	}
	if upstreamCalls != 0 {
		t.Fatalf("denied call reached upstream: %d", upstreamCalls)
	}

	allowed := &executorCredentialResolver{secret: []byte("secret")}
	outcome, err := connection.Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      executorTestTool(t, "secured"),
			Arguments: arguments,
		},
		allowed,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("Outcome kind = %q", outcome.Kind())
	}
	if allowed.calls != 1 {
		t.Fatalf("allowed resolver calls = %d, want 1", allowed.calls)
	}
	if upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls)
	}
}

type executorCredentialResolver struct {
	secret []byte
	err    error
	calls  int
}

func (r *executorCredentialResolver) Resolve(
	context.Context,
	credentialdomain.Reference,
) ([]byte, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return append([]byte(nil), r.secret...), nil
}

func TestConnectionMapsMCPErrorResultToToolFailureOutcome(t *testing.T) {
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
	outcome, err := connection.Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      executorTestTool(t, "fail"),
			Arguments: arguments,
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		outcome.FailureSource() != invocationdomain.FailureSourceTool ||
		outcome.FailureCode() != invocationdomain.FailureToolError {
		t.Fatalf(
			"outcome = kind %q source %q code %q",
			outcome.Kind(),
			outcome.FailureSource(),
			outcome.FailureCode(),
		)
	}
	if outcome.Message() != "provider rejected request" {
		t.Fatalf("Message() = %q", outcome.Message())
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
