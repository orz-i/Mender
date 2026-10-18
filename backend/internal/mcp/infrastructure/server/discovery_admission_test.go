package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// Unselected tools, verified-orphan facades and known-unavailable Sandbox
// suites are not selectable MCP search hits. Their persisted bindings/ToolIDs
// are not silently deleted by discovery.
func TestSourceFirstAdmissionHidesUnclaimedAndUnavailableProviders(t *testing.T) {
	t.Parallel()
	f := newServerFixture(t)
	unknownID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000021")
	offlineID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000022")
	unboundID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000023")
	f.tools = []catalogdomain.Tool{
		testCanonicalTool(t, "tool_00000000000000000000000000000021", f.provider.ID(), "workspace.read", "read", "Read file", "Read workspace file"),
		testCanonicalTool(t, "tool_00000000000000000000000000000022", unknownID, "workspace.read", "read", "Read file", "Read workspace file"),
		testCanonicalTool(t, "tool_00000000000000000000000000000023", offlineID, "workspace.read", "read", "Read file", "Read workspace file"),
		testCanonicalTool(t, "tool_00000000000000000000000000000024", unboundID, "agent.delegate", "delegate", "Delegate to Agent", "Submit agent task"),
	}
	profile := f.profile(t, profiledomain.ExposureDynamic, []profiledomain.ToolBindingSpec{
		{ToolID: f.tools[0].ID()},
		{ToolID: f.tools[1].ID()},
		{ToolID: f.tools[2].ID()},
	})
	a := f.adapter(t, profile)
	offline := true
	a.deps.ResolveSources = func(_ context.Context, ids []catalogdomain.ProviderID) (map[catalogdomain.ProviderID]ToolSource, error) {
		available := map[catalogdomain.ProviderID]ToolSource{
			f.provider.ID(): {Kind: "provider", Name: "Configured Workspace"},
			unknownID:       {Kind: "unclaimed", Name: "Unclaimed source"},
			offlineID:       {Kind: "sandbox", Name: "Stopped Workspace", ResourceID: "sbx_offline", Unavailable: offline},
		}
		resolved := make(map[catalogdomain.ProviderID]ToolSource, len(ids))
		for _, id := range ids {
			resolved[id] = available[id]
		}
		return resolved, nil
	}
	client := connectClient(t, a)
	sources := sourceHeaders(t, callSourceSearch(t, client, map[string]any{"query": "workspace"}))
	if len(sources) != 1 || sources[0].(map[string]any)["sourceName"] != "Configured Workspace" {
		t.Fatalf("source view exposed orphan/offline source: %#v", sources)
	}
	groups := callSourceSearch(t, client, map[string]any{"view": "tools", "query": "read"})["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["source"].(map[string]any)["sourceId"] != f.provider.ID().String() {
		t.Fatalf("cross-source search exposed orphan/offline ToolIDs: %#v", groups)
	}
	for _, hidden := range []catalogdomain.ProviderID{unknownID, offlineID, unboundID} {
		header := sourceHeaders(t, callSourceSearch(t, client, map[string]any{"view": "sources", "sourceId": hidden.String()}))
		if len(header) != 0 {
			t.Fatalf("hidden sourceId %s disclosed: %#v", hidden, header)
		}
		result := callSourceSearch(t, client, map[string]any{"view": "tools", "sourceId": hidden.String()})
		if got, ok := result["tools"].([]any); !ok || len(got) != 0 {
			t.Fatalf("hidden source %s disclosed tools: %#v", hidden, result)
		}
	}
	// Describe is part of discovery. Even if a historical bound ToolID is
	// known, orphaned/offline sources must not expose their tool schemas.
	for _, hidden := range []catalogdomain.Tool{f.tools[1], f.tools[2], f.tools[3]} {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
			Name: metaDescribeName, Arguments: map[string]any{"toolId": hidden.ID().String()},
		})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("hidden ToolID %s unexpectedly described: %+v", hidden.ID(), result)
		}
	}
	described, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: metaDescribeName, Arguments: map[string]any{"toolId": f.tools[0].ID().String()},
	})
	if err != nil || described.IsError {
		t.Fatalf("available supplied ToolID not describable: %+v / %v", described, err)
	}
	// The Profile binding for hidden tools is retained, not silently revoked.
	for _, id := range []catalogdomain.ToolID{f.tools[1].ID(), f.tools[2].ID()} {
		if !profile.Contains(id) {
			t.Fatalf("discovery unexpectedly revoked ToolID %s", id)
		}
	}
	// The source-wide known unavailability is fingerprinted: changes invalidate
	// an already-issued cursor, even though Catalog/Profile revisions are stable.
	offline = false
	described, err = client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: metaDescribeName, Arguments: map[string]any{"toolId": f.tools[2].ID().String()},
	})
	if err != nil || described.IsError {
		t.Fatalf("restored source did not restore description: %+v / %v", described, err)
	}
	page := callSourceSearch(t, client, map[string]any{"view": "sources", "limit": 1})
	cursor, ok := page["nextCursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("expected cursor with two discoverable sources: %#v", page)
	}
	offline = true
	expectSearchRejected(t, client, map[string]any{"view": "sources", "limit": 1, "cursor": cursor})
	if got := sourceHeaders(t, callSourceSearch(t, client, map[string]any{"query": "Stopped"})); len(got) != 0 {
		t.Fatalf("stopped source remained selectable: %#v", got)
	}
	if got := sourceHeaders(t, callSourceSearch(t, client, map[string]any{"query": "delegate"})); len(got) != 0 {
		t.Fatalf("unbound Agent became visible: %#v", got)
	}
}

// Even standard flat tools/list must not advertise known-unavailable or
// unclaimed Provider tools. The Profile binding itself is not mutated.
func TestDirectProjectionDoesNotExportUnclaimedOrUnavailableTools(t *testing.T) {
	t.Parallel()
	for i, src := range []ToolSource{
		{Kind: "unclaimed", Name: "Unclaimed source"},
		{Kind: "sandbox", Name: "Stopped Workspace", ResourceID: "sbx_fixture", Unavailable: true},
		{Kind: "provider", Name: "Verified service"},
	} {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			f := newServerFixture(t)
			tool := f.tools[0]
			profile := f.profile(t, profiledomain.ExposureDirect, []profiledomain.ToolBindingSpec{
				{ToolID: tool.ID(), Direct: true},
			})
			a, err := New(context.Background(), profile, Dependencies{
				DescribeTool: func(_ context.Context, id catalogdomain.ToolID) (catalogdomain.Tool, error) {
					if id != tool.ID() {
						return catalogdomain.Tool{}, fmt.Errorf("unexpected ToolID")
					}
					return tool, nil
				},
				GetProvider: func(_ context.Context, id catalogdomain.ProviderID) (catalogdomain.Provider, error) {
					if id != f.provider.ID() {
						return catalogdomain.Provider{}, fmt.Errorf("unexpected ProviderID")
					}
					return f.provider, nil
				},
				ResolveSources: func(context.Context, []catalogdomain.ProviderID) (map[catalogdomain.ProviderID]ToolSource, error) {
					return map[catalogdomain.ProviderID]ToolSource{f.provider.ID(): src}, nil
				},
				InvokeTool: func(context.Context, invocationapplication.InvokeToolRequest) (invocationapplication.InvokeToolResponse, error) {
					return invocationapplication.InvokeToolResponse{}, fmt.Errorf("not invoked")
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			session := connectClient(t, a)
			listed, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if i == 2 {
				want = 1
			}
			if len(listed.Tools) != want {
				t.Fatalf("direct tools = %#v, expected %d", listed.Tools, want)
			}
			if !profile.Contains(tool.ID()) {
				t.Fatalf("export unexpectedly removed binding %s", tool.ID())
			}
		})
	}
}

// A Source ID with a verified Kind but empty resource attribution must not
// become a guessed Agent/Sandbox source.
func TestSourceDiscoverableRequiresExactResourceIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		source ToolSource
		want   bool
	}{
		{ToolSource{Kind: "agent", Name: "Configured Agent"}, false},
		{ToolSource{Kind: "sandbox", Name: "Workspace"}, false},
		{ToolSource{Kind: "provider", Name: "Configured MCP"}, true},
		{ToolSource{Kind: "agent", Name: "OpenCode", ResourceID: "agent_verified"}, true},
		{ToolSource{Kind: "agent", Name: "OpenCode", ResourceID: "agent_verified", Unavailable: true}, false},
		{ToolSource{Kind: "unclaimed", Name: "Neutral"}, false},
	} {
		if got := sourceDiscoverable(tc.source); got != tc.want {
			t.Fatalf("source %+v discoverable=%v, want %v", tc.source, got, tc.want)
		}
	}
}
