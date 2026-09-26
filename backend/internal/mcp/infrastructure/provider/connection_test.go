package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestConnectionDiscoversUpstreamTools(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := mcp.NewServer(
		&mcp.Implementation{Name: "upstream-test", Version: "1.0.0"},
		nil,
	)
	server.AddTool(
		&mcp.Tool{
			Name:         "echo",
			Title:        "Echo",
			Description:  "Echo input",
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}}}`),
			OutputSchema: json.RawMessage(`{"type":"object"}`),
		},
		func(
			context.Context,
			*mcp.CallToolRequest,
		) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		},
	)

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

	provider := testProvider(t)
	tools, err := connection.DiscoverTools(ctx, provider)
	if err != nil {
		t.Fatalf("DiscoverTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("DiscoverTools() = %+v", tools)
	}
	if tools[0].SourceKey != "echo" || tools[0].Title != "Echo" {
		t.Fatalf("discovered tool = %+v", tools[0])
	}
}

func testProvider(t *testing.T) catalogdomain.Provider {
	t.Helper()

	id, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	namespace, _ := catalogdomain.NewNamespace("upstream")
	provider, err := catalogdomain.NewProvider(
		id,
		namespace,
		"Upstream",
		"",
		testNow(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
