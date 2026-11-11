package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SourceHeader is the compact header for a discovery source.
type SourceHeader struct {
	SourceID     string `json:"sourceId"`
	SourceKind   string `json:"sourceKind"`
	SourceName   string `json:"sourceName"`
	ToolCount    int    `json:"toolCount"`
	ResourceID   string `json:"resourceId,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
}

// ToolItem is the concise representation of a tool in tool views.
type ToolItem struct {
	ToolID string `json:"toolId"`
	Name   string `json:"name"`
	Title  string `json:"title,omitempty"`
}

// GroupItem represents a source and its query-matched tools.
type GroupItem struct {
	Source SourceHeader `json:"source"`
	Tools  []ToolItem   `json:"tools"`
}

type searchInput struct {
	View     *string `json:"view"`
	Query    string  `json:"query"`
	SourceID string  `json:"sourceId"`
	Cursor   string  `json:"cursor"`
	Limit    *int    `json:"limit"`
}

func (a *Adapter) addMetaTools() {
	a.server.AddTool(
		&mcp.Tool{
			Name:        metaSearchName,
			Title:       "Search Mender tools",
			Description: "Find Profile-visible tool sources by Agent, Workspace or service name (default view=sources). Then request view=tools with sourceId to list tools. For a tool name across sources use view=tools and query. Describe a toolId for its schema before calling it.",
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"view":{"type":"string","enum":["sources","tools"],"description":"Default sources; tools lists tools within a source or searches across sources."},"query":{"type":"string","description":"Optional for sources or tools within a source; required for cross-source tool lookup."},"sourceId":{"type":"string","description":"Stable sourceId returned by source discovery."},"cursor":{"type":"string","description":"Opaque nextCursor from the previous page."},"limit":{"type":"integer","minimum":1,"maximum":100}}}`),
		},
		a.searchHandler,
	)

	a.server.AddTool(
		&mcp.Tool{
			Name:        metaDescribeName,
			Title:       "Describe Mender tool",
			Description: "Describe one tool visible in this Mender profile",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"toolId":{"type":"string"}},"required":["toolId"]}`),
		},
		a.describeHandler,
	)

	a.server.AddTool(
		&mcp.Tool{
			Name:        metaCallName,
			Title:       "Call Mender tool",
			Description: "Invoke one tool visible in this Mender profile",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"toolId":{"type":"string"},"arguments":{"type":"object"}},"required":["toolId","arguments"]}`),
		},
		a.callHandler,
	)
}

func (a *Adapter) searchHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return a.typedSearch(ctx, request)
}

func (a *Adapter) describeHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return a.typedDescribe(ctx, request)
}

func (a *Adapter) callHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return a.typedCall(ctx, request)
}

func structuredResult(value any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode MCP structured result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(raw)},
		},
		StructuredContent: value,
	}, nil
}
