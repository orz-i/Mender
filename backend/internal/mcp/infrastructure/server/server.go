package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
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

// Dependencies are protocol-independent operations consumed by the MCP server adapter.
type Dependencies struct {
	DescribeTool func(context.Context, catalogdomain.ToolID) (catalogdomain.Tool, error)
	GetProvider  func(context.Context, catalogdomain.ProviderID) (catalogdomain.Provider, error)
	SearchTools  func(context.Context, catalogapplication.SearchToolsRequest) ([]catalogapplication.ToolSearchResult, error)
	InvokeTool   func(context.Context, invocationapplication.InvokeToolRequest) (invocationapplication.InvokeToolResponse, error)
}

// Adapter projects one Profile as an MCP server.
type Adapter struct {
	profile           profiledomain.Profile
	invocationContext governancedomain.Context
	deps              Dependencies
	server            *mcp.Server
	tools             map[catalogdomain.ToolID]catalogdomain.Tool
}

// New constructs a Profile-scoped MCP server.
func New(ctx context.Context, profile profiledomain.Profile, deps Dependencies) (*Adapter, error) {
	if deps.DescribeTool == nil || deps.GetProvider == nil || deps.SearchTools == nil || deps.InvokeTool == nil {
		return nil, fmt.Errorf("build MCP server: all dependencies are required")
	}

	profileScope, err := governancedomain.ProfileScope(profile.ID().String())
	if err != nil {
		return nil, fmt.Errorf("build MCP server: profile scope: %w", err)
	}
	principal, err := governancedomain.NewPrincipal(
		governancedomain.PrincipalService,
		"profile:"+profile.ID().String(),
	)
	if err != nil {
		return nil, fmt.Errorf("build MCP server: principal: %w", err)
	}
	invocationContext, err := governancedomain.NewContext(
		governancedomain.OriginMCP,
		principal,
		profileScope,
	)
	if err != nil {
		return nil, fmt.Errorf("build MCP server: invocation context: %w", err)
	}

	adapter := &Adapter{
		profile:           profile,
		invocationContext: invocationContext,
		deps:              deps,
		server: mcp.NewServer(
			&mcp.Implementation{Name: serverName, Version: serverVersion},
			nil,
		),
		tools: make(map[catalogdomain.ToolID]catalogdomain.Tool),
	}

	for _, toolID := range profile.ToolIDs() {
		tool, err := deps.DescribeTool(ctx, toolID)
		if err != nil {
			return nil, fmt.Errorf("build MCP server: describe tool %s: %w", toolID, err)
		}
		adapter.tools[toolID] = tool
	}

	if profile.Mode() == profiledomain.ExposureDirect || profile.Mode() == profiledomain.ExposureHybrid {
		if err := adapter.addDirectTools(ctx); err != nil {
			return nil, err
		}
	}
	if profile.Mode() == profiledomain.ExposureDynamic || profile.Mode() == profiledomain.ExposureHybrid {
		adapter.addMetaTools()
	}

	return adapter, nil
}

// MCPServer returns the underlying SDK server for transport wiring and tests.
func (a *Adapter) MCPServer() *mcp.Server {
	return a.server
}

func (a *Adapter) addDirectTools(ctx context.Context) error {
	seenNames := make(map[string]catalogdomain.ToolID)
	for _, selectedID := range a.profile.DirectToolIDs() {
		tool, ok := a.tools[selectedID]
		if !ok {
			return fmt.Errorf("build MCP server: direct Tool %s missing from profile projection", selectedID)
		}
		provider, err := a.deps.GetProvider(ctx, tool.ProviderID())
		if err != nil {
			return fmt.Errorf("build MCP server: get provider %s: %w", tool.ProviderID(), err)
		}

		presentation := effectivePresentation(a.profile, tool)
		name := exportName(
			provider.Namespace().String(),
			presentation.name,
			tool.ID(),
		)
		if existing, collision := seenNames[name]; collision && existing != tool.ID() {
			return fmt.Errorf("build MCP server: exported tool name collision %q", name)
		}
		seenNames[name] = tool.ID()

		mcpTool, err := exportedTool(name, presentation, tool)
		if err != nil {
			return fmt.Errorf("build MCP server: export %s: %w", tool.ID(), err)
		}

		toolID := tool.ID()
		a.server.AddTool(mcpTool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			response, err := a.deps.InvokeTool(ctx, invocationapplication.InvokeToolRequest{
				ToolID:    toolID,
				Arguments: request.Params.Arguments,
				Context:   a.invocationContext,
			})
			if err != nil {
				return nil, err
			}
			return mcpResult(response.Outcome)
		})
	}

	return nil
}

type toolPresentation struct {
	name        string
	title       string
	description string
}

func effectivePresentation(
	profile profiledomain.Profile,
	tool catalogdomain.Tool,
) toolPresentation {
	result := toolPresentation{
		name:        tool.Name(),
		title:       tool.Title(),
		description: tool.Description(),
	}
	binding, ok := profile.Binding(tool.ID())
	if !ok {
		return result
	}
	if binding.Alias() != "" {
		result.name = binding.Alias()
	}
	if binding.TitleOverride() != "" {
		result.title = binding.TitleOverride()
	}
	if binding.DescriptionOverride() != "" {
		result.description = binding.DescriptionOverride()
	}
	return result
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
