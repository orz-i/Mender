package server

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

const typedProfileID = "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type typedMCPFixture struct {
	mu       sync.Mutex
	snapshot supply.Snapshot
	tools    map[string]catalog.Tool
	invoked  []invocation.InvokeToolRequest
}

func (f *typedMCPFixture) Project(_ context.Context, id string) (supply.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != typedProfileID {
		return supply.Snapshot{}, supply.ErrInvalidSupply
	}
	return f.snapshot, nil
}

func (f *typedMCPFixture) describe(_ context.Context, id catalog.ToolID) (catalog.Tool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tool, ok := f.tools[id.String()]
	if !ok {
		return catalog.Tool{}, ErrToolOutsideProfile
	}
	return tool, nil
}

func (f *typedMCPFixture) invoke(_ context.Context, req invocation.InvokeToolRequest) (invocation.InvokeToolResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invoked = append(f.invoked, req)
	content, err := invocationdomain.NewJSONValue([]byte(`{"success":true}`))
	if err != nil {
		return invocation.InvokeToolResponse{}, err
	}
	outcome, err := invocationdomain.Complete(content)
	return invocation.InvokeToolResponse{Outcome: outcome}, err
}

func newTypedMCPFixture(t *testing.T) *typedMCPFixture {
	t.Helper()
	nativeToolID := ""
	for _, entry := range mustNativeTools(t) {
		if entry.SourceKey == "workspace.read" {
			nativeToolID = entry.ToolID.String()
		}
	}
	if nativeToolID == "" {
		t.Fatal("missing pinned native ToolID")
	}
	f := &typedMCPFixture{tools: make(map[string]catalog.Tool)}
	for _, item := range []struct {
		kind                         catalog.SourceKind
		id, key, toolID, title, name string
	}{
		{catalog.SourceAgent, "agent_11111111111111111111111111111111", "agent.delegate", "tool_11111111111111111111111111111111", "Agent A", "delegate"},
		{catalog.SourceAgent, "agent_22222222222222222222222222222222", "agent.delegate", "tool_22222222222222222222222222222222", "Agent B", "delegate"},
		{catalog.SourceNative, "workspace", "workspace.read", nativeToolID, "Workspace", "read"},
		{catalog.SourceProvider, "prov_33333333333333333333333333333333", "provider.search", "tool_33333333333333333333333333333333", "AnySearch", "search"},
	} {
		source, err := catalog.NewSourceRef(item.kind, item.id)
		if err != nil {
			t.Fatal(err)
		}
		id, err := catalog.ParseToolID(item.toolID)
		if err != nil {
			t.Fatal(err)
		}
		schema, err := catalog.NewSchema([]byte(`{"type":"object"}`))
		if err != nil {
			t.Fatal(err)
		}
		tool, err := catalog.NewToolFromSource(id, source, catalog.ToolSpec{
			SourceKey: item.key, Name: item.name, Title: item.title, Description: "Typed tool", InputSchema: schema,
		}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		f.tools[item.toolID] = tool
	}
	for _, spec := range []struct {
		source, kind, name string
		ids                []string
	}{
		{"agent:agent_11111111111111111111111111111111", "agent", "Agent A", []string{"tool_11111111111111111111111111111111"}},
		{"agent:agent_22222222222222222222222222222222", "agent", "Agent B", []string{"tool_22222222222222222222222222222222"}},
		{"native:workspace", "native", "Workspace", []string{nativeToolID}},
		{"provider:prov_33333333333333333333333333333333", "provider", "AnySearch", []string{"tool_33333333333333333333333333333333"}},
	} {
		src := supply.Source{SourceID: spec.source, SourceKind: spec.kind, SourceName: spec.name}
		for _, id := range spec.ids {
			tool := f.tools[id]
			src.Tools = append(src.Tools, supply.Tool{ToolID: id, SourceID: spec.source, SourceKey: tool.SourceKey(), Name: tool.Name(), Title: tool.Title(), Description: tool.Description(), Direct: true})
		}
		f.snapshot.Sources = append(f.snapshot.Sources, src)
	}
	sort.Slice(f.snapshot.Sources, func(i, j int) bool { return f.snapshot.Sources[i].SourceID < f.snapshot.Sources[j].SourceID })
	f.snapshot.ProfileID = typedProfileID
	f.snapshot.Mode = "hybrid"
	f.snapshot.Direct = []supply.Export{
		{ToolID: "tool_11111111111111111111111111111111", Name: "agent-11111111111111111111111111111111__delegate", SourceID: "agent:agent_11111111111111111111111111111111"},
		{ToolID: nativeToolID, Name: "workspace__read", SourceID: "native:workspace"},
	}
	return f
}

func mustNativeTools(t *testing.T) []curated.NativeTool {
	t.Helper()
	// Stage B registry fixes the ToolIDs independent of any Sandbox.
	tools, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	return tools
}

func (f *typedMCPFixture) newServer(t *testing.T) *Adapter {
	t.Helper()
	adapter, err := NewTyped(context.Background(), typedProfileID, TypedDependencies{Profiles: f, DescribeTool: f.describe, InvokeTool: f.invoke})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func typedCall(t *testing.T, session *mcp.ClientSession, name string, args any) map[string]any {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("typed MCP call %s: %v %+v", name, err, result)
	}
	value, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("unexpected typed MCP body %T", result.StructuredContent)
	}
	return value
}

func TestTypedMCPUsesSourceRefWithoutSyntheticProviderAndPreservesDirectAndMeta(t *testing.T) {
	f := newTypedMCPFixture(t)
	client := connectClient(t, f.newServer(t))
	names := listToolNames(t, client)
	if len(names) != 5 {
		t.Fatalf("hybrid Tool List: %v", names)
	}
	sources := typedCall(t, client, metaSearchName, map[string]any{"view": "sources", "limit": 10})["sources"].([]any)
	if len(sources) != 4 {
		t.Fatalf("typed source count: %#v", sources)
	}
	for _, s := range sources {
		header := s.(map[string]any)
		if header["sourceId"] == "" || header["sourceKind"] == "sandbox" {
			t.Fatalf("synthetic Provider/Sandbox Source: %#v", header)
		}
	}
	agents := typedCall(t, client, metaSearchName, map[string]any{"view": "tools", "query": "delegate", "limit": 10})["groups"].([]any)
	if len(agents) != 2 {
		t.Fatalf("same-name Agent IDs collapsed: %#v", agents)
	}
	native := typedCall(t, client, metaSearchName, map[string]any{"view": "tools", "sourceId": "native:workspace", "limit": 10})
	if native["source"].(map[string]any)["sourceKind"] != "native" {
		t.Fatalf("native source leaked a Provider identity: %+v", native)
	}
	nativeToolID := ""
	// Look up by SourceRef rather than source ordering.
	for _, src := range f.snapshot.Sources {
		if src.SourceID == "native:workspace" {
			nativeToolID = src.Tools[0].ToolID
		}
	}
	if nativeToolID == "" {
		t.Fatal("no native Workspace Tool in profile")
	}
	description := typedCall(t, client, metaDescribeName, map[string]any{"toolId": nativeToolID})
	if description["sourceId"] != "native:workspace" || description["sourceKind"] != "native" || description["providerId"] != nil {
		t.Fatalf("native Tool schema contained fake ProviderID: %#v", description)
	}
	provider := typedCall(t, client, metaDescribeName, map[string]any{"toolId": "tool_33333333333333333333333333333333"})
	if provider["providerId"] != "prov_33333333333333333333333333333333" ||
		provider["sourceId"] != "provider:prov_33333333333333333333333333333333" {
		t.Fatalf("real Provider lost its identity: %#v", provider)
	}
	result := typedCall(t, client, "workspace__read", map[string]any{"path": "README.md"})
	if result["success"] != true {
		t.Fatalf("direct invocation failed: %#v", result)
	}
	f.mu.Lock()
	if len(f.invoked) != 1 || f.invoked[0].ToolID.String() != nativeToolID ||
		f.invoked[0].Context.Scope().Kind() != governance.ScopeProfile ||
		f.invoked[0].Context.Scope().Reference() != typedProfileID {
		t.Fatalf("direct invoke bypassed canonical Profile context: %#v", f.invoked)
	}
	f.mu.Unlock()
}

func TestTypedMCPRevokedDirectAndCursorFailClosed(t *testing.T) {
	f := newTypedMCPFixture(t)
	client := connectClient(t, f.newServer(t))
	page := typedCall(t, client, metaSearchName, map[string]any{"view": "sources", "limit": 1})
	cursor := page["nextCursor"].(string)
	if cursor == "" {
		t.Fatal("missing cursor before grant revocation")
	}
	hiddenSource := "native:workspace"
	f.mu.Lock()
	sources := f.snapshot.Sources[:0]
	for _, s := range f.snapshot.Sources {
		if s.SourceID != hiddenSource {
			sources = append(sources, s)
		}
	}
	f.snapshot.Sources = sources
	f.snapshot.Direct = f.snapshot.Direct[:1]
	f.snapshot.ProfileRevision++
	f.mu.Unlock()
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "workspace__read", Arguments: map[string]any{"path": "a"},
	}); err == nil {
		t.Fatal("revoked native Direct Tool remained callable")
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: metaSearchName, Arguments: map[string]any{"view": "sources", "limit": 1, "cursor": cursor},
	}); err == nil {
		t.Fatal("source-first cursor survived grant revocation")
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: metaDescribeName, Arguments: map[string]any{"toolId": f.snapshot.Direct[0].ToolID /* remains Agent */},
	}); err != nil {
		t.Fatalf("unrevoked Agent metadata lost: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.invoked) != 0 {
		t.Fatal("revoked Tool reached Invocation")
	}
}

func TestTypedMCPRejectsForgedProjectionIdentityAndMismatchedCatalog(t *testing.T) {
	f := newTypedMCPFixture(t)
	if _, err := NewTyped(context.Background(), typedProfileID, TypedDependencies{Profiles: nil, DescribeTool: f.describe, InvokeTool: f.invoke}); err == nil {
		t.Fatal("missing authoritative typed Profile reader accepted")
	}
	f.mu.Lock()
	f.snapshot.ProfileID = "prof_ffffffffffffffffffffffffffffffff"
	f.mu.Unlock()
	if _, err := NewTyped(context.Background(), typedProfileID, TypedDependencies{Profiles: f, DescribeTool: f.describe, InvokeTool: f.invoke}); err == nil {
		t.Fatal("wrong Profile scope projected")
	}
	f.mu.Lock()
	f.snapshot.ProfileID = typedProfileID
	tool := f.tools["tool_11111111111111111111111111111111"]
	delete(f.tools, tool.ID().String())
	f.mu.Unlock()
	if _, err := NewTyped(context.Background(), typedProfileID, TypedDependencies{Profiles: f, DescribeTool: f.describe, InvokeTool: f.invoke}); !errors.Is(err, ErrToolOutsideProfile) {
		t.Fatalf("stale Catalog identity did not fail closed: %v", err)
	}
}
