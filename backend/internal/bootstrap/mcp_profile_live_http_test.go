package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialapp "github.com/orz-i/Mender/backend/internal/credential/application"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	profilesqlite "github.com/orz-i/Mender/backend/internal/profile/infrastructure/sqlite"
	tunnelapp "github.com/orz-i/Mender/backend/internal/tunnel/application"
)

type liveMCPFixture struct {
	runtime      *Runtime
	profile      profiledomain.Profile
	server       MCPServerInstance
	dbPath       string
	ids          map[string]catalogdomain.ToolID
	session      *mcp.ClientSession
	callCounts   *[2]atomic.Int32
	tunnelRunner *fakeTunnelRunner
}

func newLiveMCPFixture(t *testing.T, mode profiledomain.ExposureMode, useTunnel ...bool) *liveMCPFixture {
	t.Helper()
	ctx := context.Background()
	counts := new([2]atomic.Int32)
	upstream := mcp.NewServer(&mcp.Implementation{Name: "profile-hot-refresh-fixture", Version: "1"}, nil)
	for index, name := range []string{"echo_alpha", "echo_beta"} {
		index := index
		upstream.AddTool(&mcp.Tool{Name: name, Title: name, Description: name,
			InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`)},
			func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				counts[index].Add(1)
				return &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}, nil
			})
	}
	remote := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	t.Cleanup(remote.Close)
	dbPath := filepath.Join(t.TempDir(), "mender.db")
	options := RuntimeOptions{DatabasePath: dbPath}
	var runner *fakeTunnelRunner
	if len(useTunnel) > 0 && useTunnel[0] {
		t.Setenv("MENDER_HOT_REFRESH_TEST_TUNNEL_KEY", "fixture-tunnel-key")
		runner = &fakeTunnelRunner{}
		options.TunnelProcessRunner = runner
	}
	runtime, err := NewRuntime(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	config, err := json.Marshal(mcpplugin.Config{Transport: mcpplugin.TransportConfig{Kind: mcpplugin.TransportStreamableHTTP, Endpoint: remote.URL}})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, runtime, ctx, catalogapp.RegisterProviderRequest{
		Namespace: "upstream", DisplayName: "Upstream",
	}, mcpplugin.ID, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = syncConfiguredProviderTest(t, runtime, ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	results, err := runtime.Catalog.SearchTools.Execute(ctx, catalogapp.SearchToolsRequest{Query: "echo", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]catalogdomain.ToolID{}
	for _, item := range results {
		ids[item.Name] = item.ToolID
	}
	if ids["echo_alpha"] == "" || ids["echo_beta"] == "" {
		t.Fatalf("unexpected upstream tools=%+v", ids)
	}
	profile, err := runtime.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "live-mcp", DisplayName: "Live MCP", Mode: mode,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = runtime.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: profile.Slug(), ToolID: ids["echo_alpha"], Direct: mode != profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner != nil {
		credential, credentialErr := runtime.Credentials.CreateEnvironment.Execute(ctx,
			credentialapp.CreateEnvironmentRequest{Namespace: "live-http", Name: "tunnel", DisplayName: "Live Test Key",
				EnvironmentVariable: "MENDER_HOT_REFRESH_TEST_TUNNEL_KEY"})
		if credentialErr != nil {
			t.Fatal(credentialErr)
		}
		tunnel, tunnelErr := runtime.Tunnels.Create.Execute(ctx, tunnelapp.CreateOpenAITunnelRequest{
			DisplayName: "Profile Live", ExternalTunnelID: "tunnel_live_http_refresh",
			RuntimeCredentialRef: credential.Reference(),
		})
		if tunnelErr != nil {
			t.Fatal(tunnelErr)
		}
		profile, err = runtime.Profiles.SetTunnel.Execute(ctx, profileapp.SetTunnelRequest{
			ProfileSlug: profile.Slug(), TunnelID: tunnel.ID(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	server, err := startMCPHTTPServerTest(t, runtime, ctx, profile.Slug(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "live-http-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.Endpoint, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return &liveMCPFixture{runtime: runtime, profile: profile, server: server, dbPath: dbPath, ids: ids, session: session, callCounts: counts, tunnelRunner: runner}
}

func (f *liveMCPFixture) toolNames(t *testing.T) []string {
	t.Helper()
	list, err := f.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(list.Tools))
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}
func (f *liveMCPFixture) searchNames(t *testing.T) []string {
	t.Helper()
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "mender_search_tools", Arguments: json.RawMessage(`{"view":"tools","query":"echo","limit":20}`)})
	if err != nil || result.IsError {
		t.Fatalf("mender_search_tools result=%+v err=%v", result, err)
	}
	body, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("unexpected search result %T", result.StructuredContent)
	}
	groups, ok := body["groups"].([]any)
	if !ok {
		t.Fatalf("missing source groups: %#v", body)
	}
	names := make([]string, 0)
	for _, group := range groups {
		item, ok := group.(map[string]any)
		if !ok {
			t.Fatalf("malformed source group=%#v", group)
		}
		tools, ok := item["tools"].([]any)
		if !ok {
			t.Fatalf("missing grouped tools: %#v", item)
		}
		for _, value := range tools {
			tool, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("malformed search tool=%#v", value)
			}
			name, ok := tool["name"].(string)
			if !ok {
				t.Fatal("missing search tool name")
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
func (f *liveMCPFixture) assertStableListener(t *testing.T) {
	t.Helper()
	servers := f.runtime.ListMCPServers()
	if len(servers) != 1 || servers[0].ServerID != f.server.ServerID || servers[0].Endpoint != f.server.Endpoint || servers[0].StartedAt != f.server.StartedAt {
		t.Fatalf("live profile update restarted MCP listener: %+v -> %+v", f.server, servers)
	}
}

func TestManagedMCPHTTPProfileMembershipRefreshDirectAndHybrid(t *testing.T) {
	for _, mode := range []profiledomain.ExposureMode{profiledomain.ExposureDirect, profiledomain.ExposureHybrid} {
		t.Run(string(mode), func(t *testing.T) {
			f := newLiveMCPFixture(t, mode)
			initial := f.toolNames(t)
			if !containsName(initial, "upstream__echo_alpha") || containsName(initial, "upstream__echo_beta") {
				t.Fatalf("initial=%v", initial)
			}
			// A completely independent SQLite store simulates owner CLI / other process.
			external, err := profilesqlite.Open(context.Background(), f.dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = external.Close() }()
			profile, err := external.GetProfileByID(context.Background(), f.profile.ID())
			if err != nil {
				t.Fatal(err)
			}
			newBinding, err := profiledomain.NewToolBinding(profiledomain.ToolBindingSpec{ToolID: f.ids["echo_beta"], Direct: true})
			if err != nil {
				t.Fatal(err)
			}
			updated, err := profile.AddBinding(newBinding)
			if err != nil {
				t.Fatal(err)
			}
			if err := external.SaveProfile(context.Background(), updated); err != nil {
				t.Fatal(err)
			}
			added := f.toolNames(t)
			if !containsName(added, "upstream__echo_alpha") || !containsName(added, "upstream__echo_beta") {
				t.Fatalf("external AddBinding not reflected: %v", added)
			}
			f.assertStableListener(t)

			// Replace the exposed name/metadata in one committed operation.
			_, err = f.runtime.Profiles.ReplaceTools.Execute(context.Background(), profileapp.ReplaceToolsRequest{
				ProfileSlug: f.profile.Slug(), Tools: []profileapp.ToolBindingInput{
					{ToolID: f.ids["echo_alpha"], Direct: true, Alias: "renamed_alpha", TitleOverride: "Renamed Alpha"},
					{ToolID: f.ids["echo_beta"], Direct: true},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			renamed := f.toolNames(t)
			if !containsName(renamed, "upstream__renamed_alpha") || containsName(renamed, "upstream__echo_alpha") {
				t.Fatalf("metadata replacement stale: %v", renamed)
			}

			_, err = f.runtime.Profiles.RemoveTool.Execute(context.Background(), profileapp.RemoveToolRequest{
				ProfileSlug: f.profile.Slug(), ToolID: f.ids["echo_alpha"],
			})
			if err != nil {
				t.Fatal(err)
			}
			after := f.toolNames(t)
			if containsName(after, "upstream__renamed_alpha") || !containsName(after, "upstream__echo_beta") {
				t.Fatalf("revoked direct exposure remained: %v", after)
			}
			previousCalls := f.callCounts[0].Load()
			response, callErr := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "upstream__renamed_alpha", Arguments: json.RawMessage(`{}`)})
			if callErr == nil && (response == nil || !response.IsError) {
				t.Fatal("revoked direct Tool remained callable")
			}
			if got := f.callCounts[0].Load(); got != previousCalls {
				t.Fatalf("revoked Tool invoked upstream: %d", got)
			}
			f.assertStableListener(t)
		})
	}
}
func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func TestManagedMCPHTTPDynamicMetaToolsReflectUpdatedMembership(t *testing.T) {
	f := newLiveMCPFixture(t, profiledomain.ExposureDynamic)
	if got := f.toolNames(t); len(got) != 3 || !containsName(got, "mender_search_tools") {
		t.Fatalf("dynamic tools/list=%v", got)
	}
	if got := f.searchNames(t); len(got) != 1 || got[0] != "echo_alpha" {
		t.Fatalf("initial search=%v", got)
	}
	_, err := f.runtime.Profiles.AddTool.Execute(context.Background(), profileapp.AddToolRequest{
		ProfileSlug: f.profile.Slug(), ToolID: f.ids["echo_beta"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.searchNames(t); len(got) != 2 || !containsName(got, "echo_beta") {
		t.Fatalf("added search=%v", got)
	}
	_, err = f.runtime.Profiles.RemoveTool.Execute(context.Background(), profileapp.RemoveToolRequest{
		ProfileSlug: f.profile.Slug(), ToolID: f.ids["echo_alpha"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.searchNames(t); len(got) != 1 || got[0] != "echo_beta" {
		t.Fatalf("revoked search=%v", got)
	}
	result, describeErr := f.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mender_describe_tool", Arguments: json.RawMessage(`{"toolId":"` + f.ids["echo_alpha"].String() + `"}`),
	})
	if describeErr == nil && (result == nil || !result.IsError) {
		t.Fatal("revoked Tool remained describable")
	}
	denied, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mender_call_tool", Arguments: json.RawMessage(`{"toolId":"` + f.ids["echo_alpha"].String() + `","arguments":{}}`),
	})
	if err != nil || denied == nil || !denied.IsError {
		t.Fatalf("revoked dynamic tool unexpectedly callable: result=%+v err=%v", denied, err)
	}
	if calls := f.callCounts[0].Load(); calls != 0 {
		t.Fatalf("revoked dynamic tool invoked upstream %d times", calls)
	}
	f.assertStableListener(t)
}

func TestManagedMCPHTTPProfileChangesDoNotRestartTunnel(t *testing.T) {
	f := newLiveMCPFixture(t, profiledomain.ExposureDynamic, true)
	if f.server.TunnelID == "" || f.tunnelRunner == nil {
		t.Fatal("fixture Tunnel was not started")
	}
	original := f.tunnelRunner.processAt(t, 0)
	if got := f.searchNames(t); len(got) != 1 || got[0] != "echo_alpha" {
		t.Fatalf("initial=%v", got)
	}
	_, err := f.runtime.Profiles.AddTool.Execute(context.Background(), profileapp.AddToolRequest{
		ProfileSlug: f.profile.Slug(), ToolID: f.ids["echo_beta"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.searchNames(t); len(got) != 2 || !containsName(got, "echo_beta") {
		t.Fatalf("membership not refreshed: %v", got)
	}
	f.assertStableListener(t)
	f.tunnelRunner.mu.Lock()
	processCount := len(f.tunnelRunner.processes)
	f.tunnelRunner.mu.Unlock()
	original.mu.Lock()
	stopped := original.stopped
	original.mu.Unlock()
	if processCount != 1 || stopped {
		t.Fatalf("profile update restarted existing Tunnel: count=%d stopped=%t", processCount, stopped)
	}
}
