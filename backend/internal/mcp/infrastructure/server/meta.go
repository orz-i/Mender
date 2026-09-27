package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

func (a *Adapter) addMetaTools() {
	a.server.AddTool(
		&mcp.Tool{
			Name:        metaSearchName,
			Title:       "Search Mender tools",
			Description: "Search tools visible in this Mender profile",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"required":["query"]}`),
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
	var input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("mender search tools: decode input: %w", err)
	}
	if input.Limit == 0 {
		input.Limit = 10
	}

	results, err := a.deps.SearchTools(ctx, catalogapplication.SearchToolsRequest{
		Query:          input.Query,
		Limit:          input.Limit,
		AllowedToolIDs: a.profile.ToolIDs(),
	})
	if err != nil {
		return nil, err
	}

	tools := make([]map[string]any, 0, len(results))
	for _, result := range results {
		tools = append(tools, map[string]any{
			"rank":        result.Rank,
			"toolId":      result.ToolID.String(),
			"providerId":  result.ProviderID.String(),
			"name":        result.Name,
			"title":       result.Title,
			"description": result.Description,
		})
	}
	return structuredResult(map[string]any{"tools": tools})
}

func (a *Adapter) describeHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		ToolID string `json:"toolId"`
	}
	if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("mender describe tool: decode input: %w", err)
	}

	toolID, err := catalogdomain.ParseToolID(input.ToolID)
	if err != nil {
		return nil, fmt.Errorf("mender describe tool: %w", err)
	}
	if !a.profile.Contains(toolID) {
		return nil, ErrToolOutsideProfile
	}

	tool, ok := a.tools[toolID]
	if !ok {
		tool, err = a.deps.DescribeTool(ctx, toolID)
		if err != nil {
			return nil, err
		}
	}

	var inputSchema any
	if err := json.Unmarshal(tool.InputSchema().Bytes(), &inputSchema); err != nil {
		return nil, fmt.Errorf("mender describe tool: input schema: %w", err)
	}
	var outputSchema any
	if schema := tool.OutputSchema(); schema != nil {
		if err := json.Unmarshal(schema.Bytes(), &outputSchema); err != nil {
			return nil, fmt.Errorf("mender describe tool: output schema: %w", err)
		}
	}

	return structuredResult(map[string]any{
		"toolId":       tool.ID().String(),
		"providerId":   tool.ProviderID().String(),
		"name":         tool.Name(),
		"title":        tool.Title(),
		"description":  tool.Description(),
		"inputSchema":  inputSchema,
		"outputSchema": outputSchema,
		"tags":         tool.Tags(),
	})
}

func (a *Adapter) callHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		ToolID    string          `json:"toolId"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("mender call tool: decode input: %w", err)
	}

	toolID, err := catalogdomain.ParseToolID(input.ToolID)
	if err != nil {
		return nil, fmt.Errorf("mender call tool: %w", err)
	}
	if len(input.Arguments) == 0 {
		input.Arguments = json.RawMessage("{}")
	}

	response, err := a.deps.InvokeTool(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: input.Arguments,
		Context:   a.invocationContext,
	})
	if err != nil {
		return nil, err
	}
	return mcpResult(response.Outcome)
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
