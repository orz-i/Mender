package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
)

func discoveredTool(tool *mcp.Tool) (catalogapplication.DiscoveredTool, error) {
	if tool == nil {
		return catalogapplication.DiscoveredTool{}, fmt.Errorf("tool must not be nil")
	}
	if strings.TrimSpace(tool.Name) == "" {
		return catalogapplication.DiscoveredTool{}, fmt.Errorf("tool name must not be empty")
	}
	if tool.InputSchema == nil {
		return catalogapplication.DiscoveredTool{}, fmt.Errorf("input schema must not be nil")
	}

	inputSchema, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return catalogapplication.DiscoveredTool{}, fmt.Errorf("marshal input schema: %w", err)
	}

	var outputSchema []byte
	if tool.OutputSchema != nil {
		outputSchema, err = json.Marshal(tool.OutputSchema)
		if err != nil {
			return catalogapplication.DiscoveredTool{}, fmt.Errorf("marshal output schema: %w", err)
		}
	}

	title := strings.TrimSpace(tool.Title)
	if title == "" && tool.Annotations != nil {
		title = strings.TrimSpace(tool.Annotations.Title)
	}

	return catalogapplication.DiscoveredTool{
		SourceKey:    tool.Name,
		Name:         tool.Name,
		Title:        title,
		Description:  tool.Description,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Tags:         []string{"mcp"},
	}, nil
}
