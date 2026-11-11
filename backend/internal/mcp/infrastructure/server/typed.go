package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/application"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
)

// TypedProfileReader provides read-only Profile/SourceRef projections.
// It is discovery data, not an execution grant; actual calls use the canonical
// Invocation pipeline, which rechecks Profile/Policy/target authority.
type TypedProfileReader interface {
	Project(context.Context, string) (supply.Snapshot, error)
}

// TypedDependencies supply verified Profile, Catalog and Invocation operations
// for the typed MCP server adapter.
type TypedDependencies struct {
	Profiles     TypedProfileReader
	DescribeTool func(context.Context, catalog.ToolID) (catalog.Tool, error)
	InvokeTool   func(context.Context, invocation.InvokeToolRequest) (invocation.InvokeToolResponse, error)
}

type typedProfileProjection struct {
	id   string
	read TypedProfileReader
}

// NewTyped constructs a Profile-scoped MCP server backed by verified typed
// Profile projection and single governed Invocation.
func NewTyped(ctx context.Context, profileID string, deps TypedDependencies) (*Adapter, error) {
	if deps.Profiles == nil || deps.DescribeTool == nil || deps.InvokeTool == nil {
		return nil, fmt.Errorf("build typed MCP server: verified Profile, Catalog and Invocation required")
	}
	if _, err := profile.ParseProfileID(profileID); err != nil {
		return nil, fmt.Errorf("build typed MCP server: %w", err)
	}
	profileScope, err := governance.ProfileScope(profileID)
	if err != nil {
		return nil, err
	}
	principal, err := governance.NewPrincipal(governance.PrincipalService, "profile:"+profileID)
	if err != nil {
		return nil, err
	}
	caller, err := governance.NewContext(governance.OriginMCP, principal, profileScope)
	if err != nil {
		return nil, err
	}
	adapter := &Adapter{
		invocationContext: caller,
		describeTool:      deps.DescribeTool,
		invokeTool:        deps.InvokeTool,
		server:            mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil),
		typed:             &typedProfileProjection{id: profileID, read: deps.Profiles},
	}
	snapshot, err := adapter.typedSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	// The existing direct export generator and mcpResult are reused. There
	// are no Provider namespace lookups or hidden source-wide Tool grants.
	if snapshot.Mode == "direct" || snapshot.Mode == "hybrid" {
		seen := make(map[string]string, len(snapshot.Direct))
		for _, direct := range snapshot.Direct {
			if direct.Name == metaSearchName || direct.Name == metaDescribeName || direct.Name == metaCallName {
				return nil, fmt.Errorf("build typed MCP server: direct name conflicts with MCP discovery")
			}
			if previous, ok := seen[direct.Name]; ok && previous != direct.ToolID {
				return nil, fmt.Errorf("build typed MCP server: colliding direct export %q", direct.Name)
			}
			seen[direct.Name] = direct.ToolID
			selected, found := snapshotTool(snapshot, direct.ToolID)
			if !found || selected.SourceID != direct.SourceID || !selected.Direct {
				return nil, fmt.Errorf("build typed MCP server: direct Tool outside verified Profile: %w", ErrToolOutsideProfile)
			}
			tool, err := adapter.verifiedTypedTool(ctx, selected)
			if err != nil {
				return nil, err
			}
			source, _ := snapshot.Source(selected.SourceID)
			presentation := toolPresentation{
				name: selected.Name, title: selected.Title, description: selected.Description,
			}
			if source.SourceName != "" {
				presentation.title = "[" + source.SourceName + "] " + presentation.title
			}
			exported, err := exportedTool(direct.Name, presentation, tool)
			if err != nil {
				return nil, err
			}
			id, name := direct.ToolID, direct.Name
			adapter.server.AddTool(exported, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				current, err := adapter.typedSnapshot(ctx)
				if err != nil {
					return nil, err
				}
				// The fixed ListTools metadata may be stale after a grant
				// revocation; execution must always fail closed.
				visible := false
				for _, e := range current.Direct {
					if e.ToolID == id && e.Name == name && e.SourceID == direct.SourceID {
						visible = true
						break
					}
				}
				if !visible {
					return nil, ErrToolOutsideProfile
				}
				return adapter.invokeTyped(ctx, id, request.Params.Arguments, current)
			})
		}
	}
	if snapshot.Mode == "dynamic" || snapshot.Mode == "hybrid" {
		adapter.addMetaTools()
	}
	return adapter, nil
}

func (a *Adapter) typedSnapshot(ctx context.Context) (supply.Snapshot, error) {
	if a.typed == nil || a.typed.read == nil {
		return supply.Snapshot{}, supply.ErrTypedSchemaRequired
	}
	s, err := a.typed.read.Project(ctx, a.typed.id)
	if err != nil {
		return supply.Snapshot{}, err
	}
	if s.ProfileID != a.typed.id || (s.Mode != "dynamic" && s.Mode != "hybrid" && s.Mode != "direct") {
		return supply.Snapshot{}, fmt.Errorf("typed MCP server: invalid Profile projection")
	}
	return s, nil
}

func snapshotTool(s supply.Snapshot, id string) (supply.Tool, bool) {
	for _, source := range s.Sources {
		for _, tool := range source.Tools {
			if tool.ToolID == id && tool.SourceID == source.SourceID {
				return tool, true
			}
		}
	}
	return supply.Tool{}, false
}

func (a *Adapter) verifiedTypedTool(ctx context.Context, selected supply.Tool) (catalog.Tool, error) {
	id, err := catalog.ParseToolID(selected.ToolID)
	if err != nil {
		return catalog.Tool{}, err
	}
	tool, err := a.describeTool(ctx, id)
	if err != nil {
		return catalog.Tool{}, err
	}
	if tool.ID() != id || tool.SourceRef().String() != selected.SourceID || tool.SourceKey() != selected.SourceKey {
		return catalog.Tool{}, ErrToolOutsideProfile
	}
	return tool, nil
}

func (a *Adapter) invokeTyped(ctx context.Context, id string, args json.RawMessage, s supply.Snapshot) (*mcp.CallToolResult, error) {
	selected, ok := snapshotTool(s, id)
	if !ok {
		return nil, ErrToolOutsideProfile
	}
	if _, err := a.verifiedTypedTool(ctx, selected); err != nil {
		return nil, err
	}
	toolID, err := catalog.ParseToolID(id)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	response, err := a.invokeTool(ctx, invocation.InvokeToolRequest{
		ToolID: toolID, Arguments: args, Context: a.invocationContext,
	})
	if err != nil {
		return nil, err
	}
	return mcpResult(response.Outcome)
}

// MCP metadata handlers accept only the reviewed bounded request shape. They
// never forward unrecognized routing fields or extra JSON values to Invocation.
func decodeTypedMCPArguments(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("typed MCP: trailing input")
	}
	return nil
}

func (a *Adapter) typedSearch(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input searchInput
	if len(request.Params.Arguments) != 0 {
		if err := decodeTypedMCPArguments(request.Params.Arguments, &input); err != nil {
			return nil, err
		}
	}
	view := "sources"
	if input.View != nil {
		view = *input.View
	}
	limit := 20
	if input.Limit != nil {
		limit = *input.Limit
	}
	s, err := a.typedSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	page, err := s.Browse(view, input.SourceID, input.Query, input.Cursor, limit)
	if err != nil {
		return nil, err
	}
	if view == "sources" {
		return structuredResult(map[string]any{"sources": page.Sources, "nextCursor": page.NextCursor})
	}
	items := make([]ToolItem, 0, len(page.Tools))
	for _, tool := range page.Tools {
		items = append(items, ToolItem{ToolID: tool.ToolID, Name: tool.Name, Title: tool.Title})
	}
	if input.SourceID != "" {
		source, exists := s.Source(input.SourceID)
		if !exists {
			return structuredResult(map[string]any{"tools": []ToolItem{}, "nextCursor": ""})
		}
		return structuredResult(map[string]any{
			"source": SourceHeader{SourceID: source.SourceID, SourceKind: source.SourceKind,
				SourceName: source.SourceName, ToolCount: len(source.Tools)},
			"tools": items, "nextCursor": page.NextCursor,
		})
	}
	groups := make([]GroupItem, 0)
	index := make(map[string]int)
	for i, item := range page.Tools {
		at, ok := index[item.SourceID]
		if !ok {
			source, exists := s.Source(item.SourceID)
			if !exists {
				return nil, ErrToolOutsideProfile
			}
			at = len(groups)
			index[item.SourceID] = at
			groups = append(groups, GroupItem{Source: SourceHeader{
				SourceID: source.SourceID, SourceKind: source.SourceKind,
				SourceName: source.SourceName, ToolCount: len(source.Tools),
			}, Tools: []ToolItem{}})
		}
		groups[at].Tools = append(groups[at].Tools, items[i])
	}
	return structuredResult(map[string]any{"groups": groups, "nextCursor": page.NextCursor})
}

func (a *Adapter) typedDescribe(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		ToolID string `json:"toolId"`
	}
	if err := decodeTypedMCPArguments(request.Params.Arguments, &input); err != nil || strings.TrimSpace(input.ToolID) == "" {
		return nil, ErrToolOutsideProfile
	}
	s, err := a.typedSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	selected, ok := snapshotTool(s, input.ToolID)
	if !ok {
		return nil, ErrToolOutsideProfile
	}
	tool, err := a.verifiedTypedTool(ctx, selected)
	if err != nil {
		return nil, err
	}
	var schema any
	if err := json.Unmarshal(tool.InputSchema().Bytes(), &schema); err != nil {
		return nil, err
	}
	var output any
	if v := tool.OutputSchema(); v != nil {
		if err := json.Unmarshal(v.Bytes(), &output); err != nil {
			return nil, err
		}
	}
	source, err := catalog.ParseSourceRef(selected.SourceID)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{
		"toolId": selected.ToolID, "sourceId": source.String(),
		"sourceKind": string(source.Kind()), "sourceKey": selected.SourceKey,
		"name": selected.Name, "title": selected.Title, "description": selected.Description,
		"inputSchema": schema, "outputSchema": output, "tags": tool.Tags(),
	}
	if source.Kind() == catalog.SourceProvider {
		fields["providerId"] = source.ID()
	}
	return structuredResult(fields)
}

func (a *Adapter) typedCall(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		ToolID    string          `json:"toolId"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := decodeTypedMCPArguments(request.Params.Arguments, &input); err != nil || strings.TrimSpace(input.ToolID) == "" {
		return nil, ErrToolOutsideProfile
	}
	s, err := a.typedSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return a.invokeTyped(ctx, input.ToolID, input.Arguments, s)
}
