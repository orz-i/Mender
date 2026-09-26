package server

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunStdio serves this Profile over MCP stdio until the context is cancelled
// or the transport closes.
func (a *Adapter) RunStdio(ctx context.Context) error {
	return a.server.Run(ctx, &mcp.StdioTransport{})
}

// StreamableHTTPHandler serves this Profile over stateless Streamable HTTP.
//
// Stateless mode is required for the MCP 2026-07-28 sessionless protocol path.
func (a *Adapter) StreamableHTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return a.server },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
		},
	)
}
