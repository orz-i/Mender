package provider

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

const (
	clientName    = "mender"
	clientVersion = "dev"
)

// Connection owns one upstream MCP ClientSession.
//
// It is both a Catalog ToolSource and, in a separate file, an Invocation
// ToolExecutor. SDK types do not escape this package.
type Connection struct {
	session *mcp.ClientSession
}

// ConnectTransport connects Mender to an MCP Transport.
//
// This entry point is also used for deterministic in-memory integration tests.
func ConnectTransport(ctx context.Context, transport mcp.Transport) (*Connection, error) {
	if transport == nil {
		return nil, fmt.Errorf("connect MCP transport: transport must not be nil")
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: clientName, Version: clientVersion},
		nil,
	)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect MCP transport: %w", err)
	}

	return &Connection{session: session}, nil
}

// Close gracefully closes the upstream MCP session.
func (c *Connection) Close() error {
	return c.session.Close()
}

// DiscoverTools implements Catalog's ToolSource port.
func (c *Connection) DiscoverTools(
	ctx context.Context,
	_ catalogdomain.Provider,
) ([]catalogapplication.DiscoveredTool, error) {
	tools := make([]catalogapplication.DiscoveredTool, 0)

	for tool, err := range c.session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("discover MCP tools: %w", err)
		}
		discovered, mapErr := discoveredTool(tool)
		if mapErr != nil {
			return nil, fmt.Errorf("discover MCP tool %q: %w", tool.Name, mapErr)
		}
		tools = append(tools, discovered)
	}

	return tools, nil
}
