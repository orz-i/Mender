package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const (
	serverName    = "mender"
	serverVersion = "dev"

	metaSearchName   = "mender_search_tools"
	metaDescribeName = "mender_describe_tool"
	metaCallName     = "mender_call_tool"
)

var (
	ErrToolOutsideProfile       = errors.New("MCP tool is outside profile")
	ErrInputRequiredUnsupported = errors.New("MCP downstream continuation is not supported yet")
	ErrTaskUnsupported          = errors.New("MCP Tasks extension is not supported yet")
)

// Adapter projects one Profile as an MCP server using verified typed projections.
type Adapter struct {
	invocationContext governancedomain.Context
	describeTool      func(context.Context, catalogdomain.ToolID) (catalogdomain.Tool, error)
	invokeTool        func(context.Context, invocationapplication.InvokeToolRequest) (invocationapplication.InvokeToolResponse, error)
	server            *mcp.Server
	typed             *typedProfileProjection
}

// MCPServer returns the underlying SDK server for transport wiring and tests.
func (a *Adapter) MCPServer() *mcp.Server {
	return a.server
}

type toolPresentation struct {
	name        string
	title       string
	description string
}

func exportedTool(
	name string,
	presentation toolPresentation,
	tool catalogdomain.Tool,
) (*mcp.Tool, error) {
	if !isObjectSchema(tool.InputSchema().Bytes()) {
		return nil, fmt.Errorf("input schema root must be an object for MCP export")
	}

	exported := &mcp.Tool{
		Name:        name,
		Title:       presentation.title,
		Description: presentation.description,
		InputSchema: json.RawMessage(tool.InputSchema().Bytes()),
	}
	if schema := tool.OutputSchema(); schema != nil {
		exported.OutputSchema = json.RawMessage(schema.Bytes())
	}
	return exported, nil
}

func exportName(
	namespace string,
	machineName string,
	toolID catalogdomain.ToolID,
) string {
	raw := namespace + "__" + machineName
	var builder strings.Builder
	builder.Grow(len(raw))
	for _, r := range raw {
		if validToolNameRune(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}

	name := builder.String()
	if len(name) <= 128 {
		return name
	}

	suffix := "_" + strings.TrimPrefix(toolID.String(), "tool_")[:12]
	maxPrefix := 128 - len(suffix)
	return name[:maxPrefix] + suffix
}

func validToolNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-' || r == '.'
}

func isObjectSchema(raw []byte) bool {
	var schema map[string]any
	return json.Unmarshal(raw, &schema) == nil && schema != nil
}

func mcpResult(outcome invocationdomain.Outcome) (*mcp.CallToolResult, error) {
	switch outcome.Kind() {
	case invocationdomain.OutcomeKindComplete:
		raw := outcome.Content().Bytes()
		var structured any
		if err := json.Unmarshal(raw, &structured); err != nil {
			return nil, fmt.Errorf("map Mender complete result: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(raw)},
			},
			StructuredContent: structured,
		}, nil

	case invocationdomain.OutcomeKindInputRequired:
		return nil, ErrInputRequiredUnsupported

	case invocationdomain.OutcomeKindTask:
		return nil, ErrTaskUnsupported

	case invocationdomain.OutcomeKindRejected:
		return mcpErrorResult(map[string]any{
			"kind":    string(outcome.Kind()),
			"code":    string(outcome.RejectionCode()),
			"message": outcome.Message(),
		})

	case invocationdomain.OutcomeKindFailed:
		return mcpErrorResult(map[string]any{
			"kind":    string(outcome.Kind()),
			"source":  string(outcome.FailureSource()),
			"code":    string(outcome.FailureCode()),
			"message": outcome.Message(),
		})

	default:
		return nil, fmt.Errorf("map Mender outcome: unknown kind %q", outcome.Kind())
	}
}

func mcpErrorResult(structured map[string]any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(structured)
	if err != nil {
		return nil, fmt.Errorf("map Mender error outcome: %w", err)
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(raw)},
		},
		StructuredContent: structured,
	}, nil
}
