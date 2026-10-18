package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func callSourceSearch(t *testing.T, session *mcp.ClientSession, args map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: metaSearchName, Arguments: args,
	})
	if err != nil || result.IsError {
		t.Fatalf("source search args=%v error=%v result=%+v", args, err, result)
	}
	body, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("unexpected search body: %T", result.StructuredContent)
	}
	return body
}

func expectSearchRejected(t *testing.T, session *mcp.ClientSession, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: metaSearchName, Arguments: args,
	})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatalf("accepted invalid search args=%v result=%+v", args, result)
	}
}

func sourceHeaders(t *testing.T, body map[string]any) []any {
	t.Helper()
	items, ok := body["sources"].([]any)
	if !ok {
		t.Fatalf("missing sources: %#v", body)
	}
	return items
}

func TestSourceFirstSearchDistinguishesAgentsWorkspaceAndProviders(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t)
	p2, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000002")
	p3, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000003")
	p4, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000004")
	fixture.tools = []catalogdomain.Tool{
		testCanonicalTool(t, "tool_00000000000000000000000000000011", fixture.provider.ID(),
			"delegate", "delegate", "Delegate to Agent", "Submit an agent task"),
		testCanonicalTool(t, "tool_00000000000000000000000000000012", p2,
			"delegate", "delegate", "Delegate to Agent", "Submit an agent task"),
		testCanonicalTool(t, "tool_00000000000000000000000000000013", p3,
			"search", "search", "General search", strings.Repeat("agent ", 400)),
		testCanonicalTool(t, "tool_00000000000000000000000000000014", p4,
			"read", "read", "Read file", "Read workspace file"),
	}
	bindings := []profiledomain.ToolBindingSpec{}
	for _, tool := range fixture.tools {
		spec := profiledomain.ToolBindingSpec{ToolID: tool.ID()}
		if tool.ProviderID() == p4 {
			spec.Alias = "rocky_read"
			spec.TitleOverride = "Rocky SSH: Read workspace file"
		}
		bindings = append(bindings, spec)
	}
	adapter := fixture.adapter(t, fixture.profile(t, profiledomain.ExposureDynamic, bindings))
	adapter.deps.ResolveSources = func(_ context.Context, ids []catalogdomain.ProviderID) (map[catalogdomain.ProviderID]ToolSource, error) {
		owners := map[catalogdomain.ProviderID]ToolSource{
			fixture.provider.ID(): {Kind: "agent", Name: "OpenCode", ResourceID: "agent_1"},
			p2:                    {Kind: "agent", Name: "Antigravity", ResourceID: "agent_2"},
			p3:                    {Kind: "provider", Name: "AnySearch", ResourceID: p3.String()},
			p4:                    {Kind: "sandbox", Name: "Rocky Workspace", ResourceID: "sandbox_4"},
		}
		out := make(map[catalogdomain.ProviderID]ToolSource, len(ids))
		for _, id := range ids {
			out[id] = owners[id]
		}
		return out, nil
	}
	client := connectClient(t, adapter)

	// Descriptions are not a reliable source signal: 400 repetitions of
	// "agent" must not surface AnySearch in the source-first default view.
	agents := sourceHeaders(t, callSourceSearch(t, client, map[string]any{"query": "agent"}))
	if len(agents) != 2 {
		t.Fatalf("agent sources = %#v", agents)
	}
	seenAgents := map[string]bool{}
	for _, entry := range agents {
		header := entry.(map[string]any)
		if header["sourceKind"] != "agent" || header["toolCount"] != float64(1) && header["toolCount"] != 1 {
			t.Fatalf("unexpected agent source: %#v", header)
		}
		seenAgents[header["sourceName"].(string)] = true
	}
	if !seenAgents["OpenCode"] || !seenAgents["Antigravity"] {
		t.Fatalf("ambiguous same-name Agents: %#v", agents)
	}

	for _, tc := range []struct {
		query string
		pid   catalogdomain.ProviderID
		name  string
	}{
		{"OpenCode", fixture.provider.ID(), "OpenCode"},
		{"Antigravity", p2, "Antigravity"},
		{"AnySearch", p3, "AnySearch"},
		{"Rocky", p4, "Rocky Workspace"},
		{"workspace", p4, "Rocky Workspace"},
		{"rocky_read", p4, "Rocky Workspace"},
	} {
		items := sourceHeaders(t, callSourceSearch(t, client, map[string]any{"query": tc.query, "view": "sources"}))
		if len(items) != 1 {
			t.Fatalf("query %q matched sources %#v", tc.query, items)
		}
		header := items[0].(map[string]any)
		if header["sourceId"] != tc.pid.String() || header["sourceName"] != tc.name {
			t.Fatalf("query %q identified wrong Provider: %#v", tc.query, header)
		}
	}

	// A cross-source lookup retains both distinct canonical ToolIDs without
	// repeating the source label in each item.
	groups := callSourceSearch(t, client, map[string]any{"query": "delegate", "view": "tools"})["groups"].([]any)
	if len(groups) != 2 {
		t.Fatalf("delegate groups = %#v", groups)
	}
	got := map[string]string{}
	for _, entry := range groups {
		group := entry.(map[string]any)
		header := group["source"].(map[string]any)
		tools := group["tools"].([]any)
		if len(tools) != 1 {
			t.Fatalf("ambiguous group = %#v", group)
		}
		got[header["sourceId"].(string)] = tools[0].(map[string]any)["toolId"].(string)
	}
	if got[fixture.provider.ID().String()] != fixture.tools[0].ID().String() ||
		got[p2.String()] != fixture.tools[1].ID().String() {
		t.Fatalf("grouped ToolIDs crossed owners: %v", got)
	}

	rocky := callSourceSearch(t, client, map[string]any{"sourceId": p4.String(), "view": "tools"})
	tools := rocky["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "rocky_read" ||
		tools[0].(map[string]any)["title"] != "Rocky SSH: Read workspace file" {
		t.Fatalf("Profile override missing: %#v", rocky)
	}
	invisibleID := "prov_00000000000000000000000000000099"
	if len(sourceHeaders(t, callSourceSearch(t, client, map[string]any{"sourceId": invisibleID}))) != 0 {
		t.Fatal("Profile-excluded Provider disclosed by sourceId")
	}
	hiddenTools := callSourceSearch(t, client, map[string]any{"sourceId": invisibleID, "view": "tools"})
	if items, ok := hiddenTools["tools"].([]any); !ok || len(items) != 0 {
		t.Fatalf("Profile-excluded tools disclosed: %#v", hiddenTools)
	}
}

func TestSourceFirstToolsPagesStaleRevisionRenameAndMalformedCursor(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t)
	fixture.tools = nil
	bindings := make([]profiledomain.ToolBindingSpec, 0, 137)
	for i := 1; i <= 137; i++ {
		tool := testCanonicalTool(t, fmt.Sprintf("tool_%032x", 2000+i),
			fixture.provider.ID(), fmt.Sprintf("item.%03d", i),
			fmt.Sprintf("read_%03d", i), fmt.Sprintf("Read file %03d", i), "Read an allowed workspace file")
		fixture.tools = append(fixture.tools, tool)
		bindings = append(bindings, profiledomain.ToolBindingSpec{ToolID: tool.ID()})
	}
	profile := fixture.profile(t, profiledomain.ExposureDynamic, bindings)
	adapter := fixture.adapter(t, profile)
	sourceName := "Bulk Workspace"
	adapter.deps.ResolveSources = func(_ context.Context, ids []catalogdomain.ProviderID) (map[catalogdomain.ProviderID]ToolSource, error) {
		return map[catalogdomain.ProviderID]ToolSource{
			fixture.provider.ID(): {Kind: "sandbox", Name: sourceName, ResourceID: "sandbox_bulk"},
		}, nil
	}
	var revision int64 = 1
	adapter.deps.CatalogRevision = func(context.Context) (int64, error) { return revision, nil }
	client := connectClient(t, adapter)

	args := map[string]any{"sourceId": fixture.provider.ID().String(), "view": "tools", "limit": 11}
	first := callSourceSearch(t, client, args)
	if len(first["tools"].([]any)) != 11 || first["nextCursor"] == "" {
		t.Fatalf("initial page = %#v", first)
	}
	firstCursor := first["nextCursor"].(string)
	seen := map[string]bool{}
	for page := 0; page < 20; page++ {
		body := callSourceSearch(t, client, args)
		for _, entry := range body["tools"].([]any) {
			id := entry.(map[string]any)["toolId"].(string)
			if seen[id] {
				t.Fatalf("repeated ToolID across pages: %s", id)
			}
			seen[id] = true
		}
		next := body["nextCursor"].(string)
		if next == "" {
			break
		}
		args["cursor"] = next
	}
	if len(seen) != 137 {
		t.Fatalf("visible Tool pagination returned %d of 137", len(seen))
	}
	expectSearchRejected(t, client, map[string]any{"view": "tools", "query": "read", "cursor": firstCursor, "limit": 11})
	expectSearchRejected(t, client, map[string]any{"view": "tools", "sourceId": fixture.provider.ID().String(), "limit": 11, "cursor": "%%%bad"})
	expectSearchRejected(t, client, map[string]any{"view": "tools", "limit": 11})
	expectSearchRejected(t, client, map[string]any{"view": "bad"})
	expectSearchRejected(t, client, map[string]any{"view": "sources", "limit": 101})
	expectSearchRejected(t, client, map[string]any{"view": "sources", "limit": 0})
	expectSearchRejected(t, client, map[string]any{"view": ""})
	expectSearchRejected(t, client, map[string]any{"view": "sources", "unsupported": true})
	expectSearchRejected(t, client, map[string]any{"view": "tools", "sourceId": "prov_00000000000000000000000000000099", "cursor": "%bad%"})

	// Renaming the real owner must invalidate previously issued cursors,
	// even though the profile bindings and canonical ToolIDs did not change.
	sourceName = "Renamed Workspace"
	expectSearchRejected(t, client, map[string]any{"sourceId": fixture.provider.ID().String(), "view": "tools", "limit": 11, "cursor": firstCursor})
	found := sourceHeaders(t, callSourceSearch(t, client, map[string]any{"query": "Renamed"}))
	if len(found) != 1 || found[0].(map[string]any)["sourceName"] != sourceName {
		t.Fatalf("renamed source not discovered: %#v", found)
	}
	// A Catalog Tool mutation invalidates the same cursor by revision.
	sourceName = "Bulk Workspace"
	revision++
	expectSearchRejected(t, client, map[string]any{"sourceId": fixture.provider.ID().String(), "view": "tools", "limit": 11, "cursor": firstCursor})

	adapter.deps.ResolveSources = func(context.Context, []catalogdomain.ProviderID) (map[catalogdomain.ProviderID]ToolSource, error) {
		return nil, errors.New("verified owner join conflicted")
	}
	expectSearchRejected(t, client, map[string]any{"query": "read", "view": "sources"})
}
