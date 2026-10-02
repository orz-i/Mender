package server

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestDirectProfileListsAndCallsOnlySelectedTools(t *testing.T) {
	t.Parallel()

	fixture := newServerFixture(t)
	profile := fixture.profile(t, profiledomain.ExposureDirect, []profiledomain.ToolSelection{
		{ToolID: fixture.tools[0].ID()},
	})
	adapter := fixture.adapter(t, profile)
	client := connectClient(t, adapter)

	names := listToolNames(t, client)
	if len(names) != 1 || names[0] != "github__create_issue" {
		t.Fatalf("tools/list = %v", names)
	}

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "github__create_issue",
		Arguments: json.RawMessage(`{"title":"bug"}`),
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true: %+v", result.Content)
	}
	if got := fixture.invokedIDs(); len(got) != 1 || got[0] != fixture.tools[0].ID() {
		t.Fatalf("invoked ToolIDs = %v", got)
	}
	invocationContext := fixture.lastInvocationContext(t)
	if invocationContext.Origin() != governancedomain.OriginMCP {
		t.Fatalf("invocation origin = %q", invocationContext.Origin())
	}
	if invocationContext.Scope().Kind() != governancedomain.ScopeProfile ||
		invocationContext.Scope().Reference() != profile.ID().String() {
		t.Fatalf(
			"invocation scope = %q/%q, want profile %q",
			invocationContext.Scope().Kind(),
			invocationContext.Scope().Reference(),
			profile.ID(),
		)
	}
}

func TestDynamicProfileExposesMetaToolsAndEnforcesMembership(t *testing.T) {
	t.Parallel()

	fixture := newServerFixture(t)
	profile := fixture.profile(t, profiledomain.ExposureDynamic, []profiledomain.ToolSelection{
		{ToolID: fixture.tools[0].ID()},
	})
	adapter := fixture.adapter(t, profile)
	client := connectClient(t, adapter)

	names := listToolNames(t, client)
	want := []string{metaCallName, metaDescribeName, metaSearchName}
	sort.Strings(want)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools/list = %v, want %v", names, want)
	}

	searchResult, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      metaSearchName,
		Arguments: json.RawMessage(`{"query":"issue","limit":10}`),
	})
	if err != nil {
		t.Fatalf("search CallTool() error = %v", err)
	}
	searchMap, ok := searchResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("search StructuredContent = %T", searchResult.StructuredContent)
	}
	listed, ok := searchMap["tools"].([]any)
	if !ok || len(listed) != 1 {
		t.Fatalf("search tools = %#v", searchMap["tools"])
	}
	lastScope := fixture.lastSearchAllowed()
	if len(lastScope) != 1 || lastScope[0] != fixture.tools[0].ID() {
		t.Fatalf("search allowed ToolIDs = %v", lastScope)
	}

	_, err = client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      metaDescribeName,
		Arguments: json.RawMessage(`{"toolId":"` + fixture.tools[2].ID().String() + `"}`),
	})
	if err == nil {
		t.Fatal("describe hidden Tool expected error")
	}

	hiddenCall, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      metaCallName,
		Arguments: json.RawMessage(`{"toolId":"` + fixture.tools[2].ID().String() + `","arguments":{}}`),
	})
	if err != nil {
		t.Fatalf("call hidden Tool transport error = %v", err)
	}
	if !hiddenCall.IsError {
		t.Fatal("call hidden Tool did not map Profile rejection to MCP Tool error")
	}
	if got := fixture.invokedIDs(); len(got) != 1 || got[0] != fixture.tools[2].ID() {
		t.Fatalf("hidden Tool did not reach authoritative Invocation boundary: %v", got)
	}

	callResult, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      metaCallName,
		Arguments: json.RawMessage(`{"toolId":"` + fixture.tools[0].ID().String() + `","arguments":{"title":"bug"}}`),
	})
	if err != nil {
		t.Fatalf("call visible Tool error = %v", err)
	}
	if callResult.IsError {
		t.Fatalf("call visible Tool returned IsError")
	}
	if got := fixture.invokedIDs(); len(got) != 2 ||
		got[0] != fixture.tools[2].ID() || got[1] != fixture.tools[0].ID() {
		t.Fatalf("invoked ToolIDs = %v", got)
	}
}

func TestProfileBindingPresentationOverridesMCPProjection(t *testing.T) {
	t.Parallel()

	fixture := newServerFixture(t)
	binding, err := profiledomain.NewToolBinding(profiledomain.ToolBindingSpec{
		ToolID:              fixture.tools[0].ID(),
		Direct:              true,
		Alias:               "open_prod_issue",
		TitleOverride:       "Open production issue",
		DescriptionOverride: "Open an issue in the production tracker.",
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := fixture.profileBindings(
		t,
		profiledomain.ExposureHybrid,
		[]profiledomain.ToolBinding{binding},
	)
	adapter := fixture.adapter(t, profile)
	client := connectClient(t, adapter)

	listed, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var direct *mcp.Tool
	for _, tool := range listed.Tools {
		if tool.Name == "github__open_prod_issue" {
			direct = tool
			break
		}
	}
	if direct == nil {
		t.Fatalf("direct tools = %#v", listed.Tools)
	}
	if direct.Title != "Open production issue" ||
		direct.Description != "Open an issue in the production tracker." {
		t.Fatalf(
			"direct presentation = title %q description %q",
			direct.Title,
			direct.Description,
		)
	}
	var canonicalInputSchema any
	if err := json.Unmarshal(fixture.tools[0].InputSchema().Bytes(), &canonicalInputSchema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(direct.InputSchema, canonicalInputSchema) {
		t.Fatalf(
			"direct InputSchema changed: got %#v want %#v",
			direct.InputSchema,
			canonicalInputSchema,
		)
	}

	searchResult, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      metaSearchName,
		Arguments: json.RawMessage(`{"query":"issue","limit":10}`),
	})
	if err != nil {
		t.Fatalf("search CallTool() error = %v", err)
	}
	searchMap, ok := searchResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("search StructuredContent = %T", searchResult.StructuredContent)
	}
	items, ok := searchMap["tools"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("search tools = %#v", searchMap["tools"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("search item = %#v", items[0])
	}
	if item["name"] != "open_prod_issue" ||
		item["title"] != "Open production issue" ||
		item["description"] != "Open an issue in the production tracker." {
		t.Fatalf("search presentation = %#v", item)
	}

	describeResult, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: metaDescribeName,
		Arguments: json.RawMessage(
			`{"toolId":"` + fixture.tools[0].ID().String() + `"}`,
		),
	})
	if err != nil {
		t.Fatalf("describe CallTool() error = %v", err)
	}
	describeMap, ok := describeResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("describe StructuredContent = %T", describeResult.StructuredContent)
	}
	if describeMap["toolId"] != fixture.tools[0].ID().String() ||
		describeMap["name"] != "open_prod_issue" ||
		describeMap["title"] != "Open production issue" ||
		describeMap["description"] != "Open an issue in the production tracker." {
		t.Fatalf("describe presentation = %#v", describeMap)
	}
	inputSchema, ok := describeMap["inputSchema"].(map[string]any)
	if !ok || inputSchema["type"] != "object" {
		t.Fatalf("describe inputSchema = %#v", describeMap["inputSchema"])
	}
}

func TestMCPDirectProjectionRejectsNormalizedNameCollision(t *testing.T) {
	t.Parallel()

	fixture := newServerFixture(t)
	first := testCanonicalTool(
		t,
		"tool_00000000000000000000000000000011",
		fixture.provider.ID(),
		"collision.first",
		"collision/name",
		"Collision first",
		"",
	)
	second := testCanonicalTool(
		t,
		"tool_00000000000000000000000000000012",
		fixture.provider.ID(),
		"collision.second",
		"collision_name",
		"Collision second",
		"",
	)
	fixture.tools = []catalogdomain.Tool{first, second}

	firstBinding, err := profiledomain.DefaultToolBinding(first.ID(), true)
	if err != nil {
		t.Fatal(err)
	}
	secondBinding, err := profiledomain.DefaultToolBinding(second.ID(), true)
	if err != nil {
		t.Fatal(err)
	}
	profile := fixture.profileBindings(
		t,
		profiledomain.ExposureDirect,
		[]profiledomain.ToolBinding{firstBinding, secondBinding},
	)

	byID := map[catalogdomain.ToolID]catalogdomain.Tool{
		first.ID():  first,
		second.ID(): second,
	}
	_, err = New(context.Background(), profile, Dependencies{
		DescribeTool: func(_ context.Context, id catalogdomain.ToolID) (catalogdomain.Tool, error) {
			return byID[id], nil
		},
		GetProvider: func(context.Context, catalogdomain.ProviderID) (catalogdomain.Provider, error) {
			return fixture.provider, nil
		},
		SearchTools: func(context.Context, catalogapplication.SearchToolsRequest) ([]catalogapplication.ToolSearchResult, error) {
			return nil, nil
		},
		InvokeTool: func(context.Context, invocationapplication.InvokeToolRequest) (invocationapplication.InvokeToolResponse, error) {
			return invocationapplication.InvokeToolResponse{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "exported tool name collision") {
		t.Fatalf("New() error = %v, want normalized exported-name collision", err)
	}
}

func TestHybridProfileCombinesDirectAndDynamicExposure(t *testing.T) {
	t.Parallel()

	fixture := newServerFixture(t)
	profile := fixture.profile(t, profiledomain.ExposureHybrid, []profiledomain.ToolSelection{
		{ToolID: fixture.tools[0].ID(), Direct: true},
		{ToolID: fixture.tools[1].ID(), Direct: false},
	})
	adapter := fixture.adapter(t, profile)
	client := connectClient(t, adapter)

	names := listToolNames(t, client)
	want := []string{
		"github__create_issue",
		metaCallName,
		metaDescribeName,
		metaSearchName,
	}
	sort.Strings(want)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools/list = %v, want %v", names, want)
	}
	for _, name := range names {
		if name == "github__list_issues" {
			t.Fatal("non-direct hybrid Tool was exported directly")
		}
	}
}

func TestMCPResultRejectsUnsupportedCanonicalVariants(t *testing.T) {
	t.Parallel()

	inputRequired, err := invocationdomain.InputRequired("req-1", "more input", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcpResult(inputRequired); !errors.Is(err, ErrInputRequiredUnsupported) {
		t.Fatalf("InputRequired error = %v", err)
	}

	task, err := invocationdomain.Task("task-1", "running", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcpResult(task); !errors.Is(err, ErrTaskUnsupported) {
		t.Fatalf("Task error = %v", err)
	}
}

func TestMCPResultMapsRejectedAndFailedOutcomesToToolErrors(t *testing.T) {
	t.Parallel()

	rejected, err := invocationdomain.Rejected(
		invocationdomain.RejectionToolOutsideScope,
		"tool is not available in this execution scope",
	)
	if err != nil {
		t.Fatal(err)
	}
	rejectedResult, err := mcpResult(rejected)
	if err != nil {
		t.Fatalf("mcpResult(rejected) error = %v", err)
	}
	if !rejectedResult.IsError {
		t.Fatal("rejected outcome did not map to MCP Tool error")
	}
	rejectedStructured, ok := rejectedResult.StructuredContent.(map[string]any)
	if !ok || rejectedStructured["kind"] != "rejected" ||
		rejectedStructured["code"] != "tool_outside_scope" {
		t.Fatalf("rejected StructuredContent = %#v", rejectedResult.StructuredContent)
	}

	failed, err := invocationdomain.Failed(
		invocationdomain.FailureSourceTool,
		invocationdomain.FailureToolError,
		"tool rejected request",
	)
	if err != nil {
		t.Fatal(err)
	}
	failedResult, err := mcpResult(failed)
	if err != nil {
		t.Fatalf("mcpResult(failed) error = %v", err)
	}
	if !failedResult.IsError {
		t.Fatal("failed outcome did not map to MCP Tool error")
	}
	failedStructured, ok := failedResult.StructuredContent.(map[string]any)
	if !ok || failedStructured["kind"] != "failed" ||
		failedStructured["source"] != "tool" || failedStructured["code"] != "tool_error" {
		t.Fatalf("failed StructuredContent = %#v", failedResult.StructuredContent)
	}
}

type serverFixture struct {
	provider catalogdomain.Provider
	tools    []catalogdomain.Tool

	mu                 sync.Mutex
	invoked            []catalogdomain.ToolID
	invocationContexts []governancedomain.Context
	searchAllowed      []catalogdomain.ToolID
}

func newServerFixture(t *testing.T) *serverFixture {
	t.Helper()

	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	namespace, _ := catalogdomain.NewNamespace("github")
	provider, err := catalogdomain.NewProvider(
		providerID,
		namespace,
		"GitHub",
		"",
		time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}

	return &serverFixture{
		provider: provider,
		tools: []catalogdomain.Tool{
			testCanonicalTool(t, "tool_00000000000000000000000000000001", providerID, "issues.create", "create_issue", "Create issue", "Create an issue"),
			testCanonicalTool(t, "tool_00000000000000000000000000000002", providerID, "issues.list", "list_issues", "List issues", "List repository issues"),
			testCanonicalTool(t, "tool_00000000000000000000000000000003", providerID, "repo.delete", "delete_repo", "Delete repository", "Delete a repository"),
		},
	}
}

func (f *serverFixture) profile(t *testing.T, mode profiledomain.ExposureMode, selections []profiledomain.ToolSelection) profiledomain.Profile {
	t.Helper()
	id, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := profiledomain.NewSlug("default")
	profile, err := profiledomain.NewProfile(id, slug, "Default", mode, selections)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func (f *serverFixture) profileBindings(
	t *testing.T,
	mode profiledomain.ExposureMode,
	bindings []profiledomain.ToolBinding,
) profiledomain.Profile {
	t.Helper()
	id, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := profiledomain.NewSlug("default")
	profile, err := profiledomain.NewProfileWithBindings(
		id,
		slug,
		"Default",
		mode,
		bindings,
	)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func (f *serverFixture) adapter(t *testing.T, profile profiledomain.Profile) *Adapter {
	t.Helper()

	byID := make(map[catalogdomain.ToolID]catalogdomain.Tool, len(f.tools))
	for _, tool := range f.tools {
		byID[tool.ID()] = tool
	}

	adapter, err := New(context.Background(), profile, Dependencies{
		DescribeTool: func(_ context.Context, id catalogdomain.ToolID) (catalogdomain.Tool, error) {
			tool, ok := byID[id]
			if !ok {
				return catalogdomain.Tool{}, catalogapplication.ErrToolNotFound
			}
			return tool, nil
		},
		GetProvider: func(_ context.Context, id catalogdomain.ProviderID) (catalogdomain.Provider, error) {
			if id != f.provider.ID() {
				return catalogdomain.Provider{}, catalogapplication.ErrProviderNotFound
			}
			return f.provider, nil
		},
		SearchTools: func(_ context.Context, request catalogapplication.SearchToolsRequest) ([]catalogapplication.ToolSearchResult, error) {
			f.mu.Lock()
			f.searchAllowed = append([]catalogdomain.ToolID(nil), request.AllowedToolIDs...)
			f.mu.Unlock()

			allowed := make(map[catalogdomain.ToolID]struct{}, len(request.AllowedToolIDs))
			for _, id := range request.AllowedToolIDs {
				allowed[id] = struct{}{}
			}
			results := make([]catalogapplication.ToolSearchResult, 0)
			for _, tool := range f.tools {
				if _, ok := allowed[tool.ID()]; !ok {
					continue
				}
				haystack := strings.ToLower(tool.Name() + " " + tool.Title() + " " + tool.Description())
				if !strings.Contains(haystack, strings.ToLower(request.Query)) {
					continue
				}
				results = append(results, catalogapplication.ToolSearchResult{
					Rank:        len(results) + 1,
					ToolID:      tool.ID(),
					ProviderID:  tool.ProviderID(),
					Name:        tool.Name(),
					Title:       tool.Title(),
					Description: tool.Description(),
				})
				if len(results) == request.Limit {
					break
				}
			}
			return results, nil
		},
		InvokeTool: func(_ context.Context, request invocationapplication.InvokeToolRequest) (invocationapplication.InvokeToolResponse, error) {
			f.mu.Lock()
			f.invoked = append(f.invoked, request.ToolID)
			f.invocationContexts = append(f.invocationContexts, request.Context)
			f.mu.Unlock()
			if request.Context.Scope().Kind() == governancedomain.ScopeProfile &&
				!profile.Contains(request.ToolID) {
				rejected, rejectErr := invocationdomain.Rejected(
					invocationdomain.RejectionToolOutsideScope,
					"tool is not available in this execution scope",
				)
				if rejectErr != nil {
					return invocationapplication.InvokeToolResponse{}, rejectErr
				}
				return invocationapplication.InvokeToolResponse{Outcome: rejected}, nil
			}
			value, err := invocationdomain.NewJSONValue(request.Arguments)
			if err != nil {
				return invocationapplication.InvokeToolResponse{}, err
			}
			result, err := invocationdomain.Complete(value)
			if err != nil {
				return invocationapplication.InvokeToolResponse{}, err
			}
			return invocationapplication.InvokeToolResponse{Outcome: result}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return adapter
}

func (f *serverFixture) invokedIDs() []catalogdomain.ToolID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]catalogdomain.ToolID(nil), f.invoked...)
}

func (f *serverFixture) lastInvocationContext(t *testing.T) governancedomain.Context {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.invocationContexts) == 0 {
		t.Fatal("no invocation contexts")
	}
	return f.invocationContexts[len(f.invocationContexts)-1]
}

func (f *serverFixture) lastSearchAllowed() []catalogdomain.ToolID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]catalogdomain.ToolID(nil), f.searchAllowed...)
}

func connectClient(t *testing.T, adapter *Adapter) *mcp.ClientSession {
	t.Helper()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := adapter.MCPServer().Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(
		&mcp.Implementation{Name: "mender-server-test", Version: "1.0.0"},
		nil,
	)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func listToolNames(t *testing.T, client *mcp.ClientSession) []string {
	t.Helper()

	result, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func testCanonicalTool(
	t *testing.T,
	rawID string,
	providerID catalogdomain.ProviderID,
	sourceKey string,
	name string,
	title string,
	description string,
) catalogdomain.Tool {
	t.Helper()

	toolID, _ := catalogdomain.ParseToolID(rawID)
	schema, _ := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   sourceKey,
			Name:        name,
			Title:       title,
			Description: description,
			InputSchema: schema,
		},
		time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}
