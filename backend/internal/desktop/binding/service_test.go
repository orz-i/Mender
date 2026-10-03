package binding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestServiceEndToEndUsesSharedRuntime(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "desktop-binding-upstream", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:         "echo",
			Title:        "Echo",
			Description:  "Echo JSON input",
			InputSchema:  json.RawMessage(`{"type":"object"}`),
			OutputSchema: json.RawMessage(`{"type":"object"}`),
		},
		func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			upstreamCalls.Add(1)
			var input map[string]any
			if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{StructuredContent: input}, nil
		},
	)
	upstreamHTTP := httptest.NewServer(
		mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return upstream },
			&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
		),
	)
	t.Cleanup(upstreamHTTP.Close)

	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	service := NewService(context.Background(), runtime)

	status, err := service.GetSystemStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ready || !status.Policy.Editable || !status.Credentials.Editable ||
		status.Policy.Mode != "configured" ||
		status.Policy.DefaultDecision != "allow" ||
		status.Policy.RuleCount != 0 {
		t.Fatalf("system status = %#v", status)
	}

	runtimeStatus, err := service.GetRuntimeGovernance()
	if err != nil {
		t.Fatal(err)
	}
	if runtimeStatus.Budget.MaxInvocations != 32 || runtimeStatus.Backend.Name == "" {
		t.Fatalf("runtime governance = %#v", runtimeStatus)
	}
	updatedRuntime, err := service.UpdateRuntimeBudget(RuntimeBudget{
		MaxInvocations:          12,
		MaxPrincipalInvocations: runtimeStatus.Budget.MaxPrincipalInvocations,
		MaxProviderInvocations:  runtimeStatus.Budget.MaxProviderInvocations,
		MaxToolInvocations:      runtimeStatus.Budget.MaxToolInvocations,
		MaxProviderActivations:  runtimeStatus.Budget.MaxProviderActivations,
		MaxExecutionTimeoutMS:   runtimeStatus.Budget.MaxExecutionTimeoutMS,
		MaxArgumentBytes:        runtimeStatus.Budget.MaxArgumentBytes,
		MaxResultBytes:          runtimeStatus.Budget.MaxResultBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedRuntime.Budget.MaxInvocations != 12 {
		t.Fatalf("updated runtime governance = %#v", updatedRuntime)
	}

	created, err := service.CreateHTTPProvider(
		"upstream",
		"Upstream",
		"",
		upstreamHTTP.URL,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("CreateHTTPProvider() error = %v", err)
	}
	if created.Provider.ProviderID == "" || created.Provider.Transport != "http" {
		t.Fatalf("created provider = %#v", created.Provider)
	}
	if created.Sync.Total != 1 {
		t.Fatalf("sync result = %#v", created.Sync)
	}

	updated, err := service.UpdateProvider(
		created.Provider.ProviderID,
		"Upstream edited",
		"Edited provider",
		"http",
		upstreamHTTP.URL,
		nil,
		nil,
		"",
		nil,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("UpdateProvider() error = %v", err)
	}
	if updated.Provider.Namespace != "upstream" ||
		updated.Provider.DisplayName != "Upstream edited" ||
		updated.Provider.Description != "Edited provider" {
		t.Fatalf("updated provider = %#v", updated.Provider)
	}

	browse, err := service.BrowseTools(100, created.Provider.ProviderID)
	if err != nil {
		t.Fatalf("BrowseTools() error = %v", err)
	}
	if len(browse.Tools) != 1 || browse.Tools[0].Name != "echo" {
		t.Fatalf("browse tools = %#v", browse.Tools)
	}

	search, err := service.SearchTools("echo", 10, "", "")
	if err != nil {
		t.Fatalf("SearchTools() error = %v", err)
	}
	if len(search.Tools) != 1 {
		t.Fatalf("search tools = %#v", search.Tools)
	}
	toolID := search.Tools[0].ToolID

	detail, err := service.GetTool(toolID)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if detail.Name != "echo" || detail.InputSchema == nil {
		t.Fatalf("tool detail = %#v", detail)
	}

	profile, err := service.CreateProfile("default", "Default", "hybrid")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if profile.Mode != "hybrid" {
		t.Fatalf("profile mode = %q", profile.Mode)
	}
	profile, err = service.AddProfileTool("default", ProfileTool{
		ToolID: toolID,
		Direct: true,
	})
	if err != nil {
		t.Fatalf("AddProfileTool() error = %v", err)
	}
	if len(profile.Tools) != 1 || !profile.Tools[0].Direct ||
		profile.Tools[0].PolicyConstraint != "inherit" ||
		profile.Tools[0].ConcurrencyConstraint != "inherit" {
		t.Fatalf("profile tools = %#v", profile.Tools)
	}

	profile, err = service.ReplaceProfileTools("default", []ProfileTool{
		{
			ToolID:                toolID,
			Direct:                true,
			Alias:                 "echo_desktop",
			TitleOverride:         "Desktop Echo",
			DescriptionOverride:   "Echo configured from the Desktop binding.",
			PolicyConstraint:      "inherit",
			ConcurrencyConstraint: "serial",
		},
	})
	if err != nil {
		t.Fatalf("ReplaceProfileTools() error = %v", err)
	}
	if len(profile.Tools) != 1 || profile.Tools[0].ToolID != toolID ||
		profile.Tools[0].Alias != "echo_desktop" ||
		profile.Tools[0].TitleOverride != "Desktop Echo" ||
		profile.Tools[0].DescriptionOverride != "Echo configured from the Desktop binding." ||
		profile.Tools[0].PolicyConstraint != "inherit" ||
		profile.Tools[0].ConcurrencyConstraint != "serial" {
		t.Fatalf("replaced profile tools = %#v", profile.Tools)
	}

	providerScoped, err := service.SearchTools(
		"echo",
		10,
		created.Provider.ProviderID,
		"",
	)
	if err != nil {
		t.Fatalf("provider SearchTools() error = %v", err)
	}
	if len(providerScoped.Tools) != 1 || providerScoped.Tools[0].ToolID != toolID {
		t.Fatalf("provider search tools = %#v", providerScoped.Tools)
	}

	scoped, err := service.SearchTools("echo", 10, "", "default")
	if err != nil {
		t.Fatalf("profile SearchTools() error = %v", err)
	}
	if len(scoped.Tools) != 1 || scoped.Tools[0].ToolID != toolID {
		t.Fatalf("profile search tools = %#v", scoped.Tools)
	}

	serverInstance, err := service.StartMCPServer("default", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("StartMCPServer() error = %v", err)
	}
	if serverInstance.Status != "running" ||
		serverInstance.Transport != "streamable-http" {
		t.Fatalf("server instance = %#v", serverInstance)
	}
	serverList := service.ListMCPServers()
	if len(serverList.Servers) != 1 ||
		serverList.Servers[0].ServerID != serverInstance.ServerID {
		t.Fatalf("server list = %#v", serverList)
	}
	restarted, err := service.RestartMCPServer(serverInstance.ServerID)
	if err != nil {
		t.Fatalf("RestartMCPServer() error = %v", err)
	}
	if restarted.ServerID == serverInstance.ServerID {
		t.Fatalf("restart reused server ID %q", restarted.ServerID)
	}
	if err := service.StopMCPServer(restarted.ServerID); err != nil {
		t.Fatalf("StopMCPServer() error = %v", err)
	}
	if len(service.ListMCPServers().Servers) != 0 {
		t.Fatal("servers remain after stop")
	}

	result, err := service.CallTool(toolID, map[string]any{"value": "desktop"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Kind != "complete" {
		t.Fatalf("result = %#v", result)
	}
	content, ok := result.Content.(map[string]any)
	if !ok || content["value"] != "desktop" {
		t.Fatalf("result content = %#v", result.Content)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls.Load())
	}

	missing, err := service.CallTool(
		"tool_ffffffffffffffffffffffffffffffff",
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("CallTool(missing) error = %v", err)
	}
	if missing.Kind != "rejected" || missing.Code != "tool_not_found" {
		t.Fatalf("missing Tool result = %#v", missing)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("missing Tool unexpectedly reached upstream; calls = %d", upstreamCalls.Load())
	}

	audit, err := service.ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(audit.Records) != 2 {
		t.Fatalf("audit records = %#v", audit.Records)
	}
	rejectedRecord := audit.Records[0]
	if rejectedRecord.PrincipalKind != "local_user" ||
		!strings.HasPrefix(rejectedRecord.PrincipalRef, "os:") ||
		rejectedRecord.Origin != "desktop" ||
		rejectedRecord.ScopeKind != "catalog" ||
		rejectedRecord.OutcomeKind != "rejected" ||
		rejectedRecord.RejectionCode != "tool_not_found" {
		t.Fatalf("rejected audit record = %#v", rejectedRecord)
	}
	completedRecord := audit.Records[1]
	if completedRecord.PrincipalKind != "local_user" ||
		!strings.HasPrefix(completedRecord.PrincipalRef, "os:") ||
		completedRecord.ResultKind != "complete" ||
		completedRecord.OutcomeKind != "complete" {
		t.Fatalf("completed audit record = %#v", completedRecord)
	}

	if err := service.DeleteProvider(created.Provider.ProviderID); !errors.Is(err, bootstrap.ErrProviderInUse) {
		t.Fatalf("DeleteProvider() error = %v, want ErrProviderInUse", err)
	}

	if _, err := service.RemoveProfileTool("default", toolID); err != nil {
		t.Fatalf("RemoveProfileTool() error = %v", err)
	}
	if err := service.DeleteProvider(created.Provider.ProviderID); err != nil {
		t.Fatalf("DeleteProvider() error = %v", err)
	}

	providers, err := service.ListProviders()
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if len(providers.Providers) != 0 {
		t.Fatalf("providers after delete = %#v", providers.Providers)
	}
}
