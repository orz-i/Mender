package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpprovider "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/provider"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestMCPProviderAndServerShareRuntimePipeline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:   filepath.Join(t.TempDir(), "mender.db"),
		DefaultTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	provider, err := runtime.Catalog.RegisterProvider.Execute(ctx, catalogapplication.RegisterProviderRequest{
		Namespace:   "upstream",
		DisplayName: "Upstream MCP",
	})
	if err != nil {
		t.Fatalf("RegisterProvider.Execute() error = %v", err)
	}

	var upstreamCalls atomic.Int32
	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "phase4-upstream", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:         "echo",
			Title:        "Echo",
			Description:  "Echo input",
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

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	upstreamSession, err := upstream.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("upstream.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = upstreamSession.Close() })

	connection, err := mcpprovider.ConnectTransport(ctx, clientTransport)
	if err != nil {
		t.Fatalf("ConnectTransport() error = %v", err)
	}

	syncResult, err := runtime.AttachMCPProvider(ctx, provider.ID(), connection)
	if err != nil {
		_ = connection.Close()
		t.Fatalf("AttachMCPProvider() error = %v", err)
	}
	if syncResult.Created != 1 || syncResult.Total != 1 {
		t.Fatalf("sync result = %+v", syncResult)
	}

	search, err := runtime.Catalog.SearchTools.Execute(ctx, catalogapplication.SearchToolsRequest{
		Query: "echo",
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("SearchTools.Execute() error = %v", err)
	}
	if len(search) != 1 {
		t.Fatalf("SearchTools results = %+v", search)
	}
	toolID := search[0].ToolID

	applicationContext, err := invocationdomain.NewInvocationContext(
		invocationdomain.OriginInternal,
		"integration-test",
		invocationdomain.CatalogExecutionScope(),
	)
	if err != nil {
		t.Fatalf("NewInvocationContext() error = %v", err)
	}
	applicationResponse, err := runtime.InvokeTool.Execute(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: []byte(`{"path":"application"}`),
		Context:   applicationContext,
	})
	if err != nil {
		t.Fatalf("InvokeTool.Execute() error = %v", err)
	}
	if applicationResponse.Outcome.Kind() != invocationdomain.ResultKindComplete {
		t.Fatalf("application result kind = %q", applicationResponse.Outcome.Kind())
	}
	auditRecord, err := runtime.audit.Get(ctx, applicationResponse.InvocationID)
	if err != nil {
		t.Fatalf("audit.Get() error = %v", err)
	}
	if auditRecord.Status() != auditdomain.StatusCompleted || auditRecord.ToolID() != toolID {
		t.Fatalf("audit record = %#v", auditRecord)
	}

	profile, err := runtime.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{
		Slug:        "default",
		DisplayName: "Default",
		Mode:        profiledomain.ExposureDirect,
	})
	if err != nil {
		t.Fatalf("CreateProfile.Execute() error = %v", err)
	}
	profile, err = runtime.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{
		ProfileSlug: profile.Slug(),
		ToolID:      toolID,
		Direct:      true,
	})
	if err != nil {
		t.Fatalf("AddTool.Execute() error = %v", err)
	}

	downstream, err := runtime.NewMCPServer(ctx, profile)
	if err != nil {
		t.Fatalf("NewMCPServer() error = %v", err)
	}
	downstreamClientTransport, downstreamServerTransport := mcp.NewInMemoryTransports()
	downstreamServerSession, err := downstream.MCPServer().Connect(ctx, downstreamServerTransport, nil)
	if err != nil {
		t.Fatalf("downstream server.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = downstreamServerSession.Close() })

	client := mcp.NewClient(
		&mcp.Implementation{Name: "phase4-downstream-client", Version: "1.0.0"},
		nil,
	)
	downstreamClient, err := client.Connect(ctx, downstreamClientTransport, nil)
	if err != nil {
		t.Fatalf("downstream client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = downstreamClient.Close() })

	listed, err := downstreamClient.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("downstream ListTools() error = %v", err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "upstream__echo" {
		t.Fatalf("downstream tools/list = %+v", listed.Tools)
	}

	mcpResult, err := downstreamClient.CallTool(ctx, &mcp.CallToolParams{
		Name:      "upstream__echo",
		Arguments: json.RawMessage(`{"path":"mcp"}`),
	})
	if err != nil {
		t.Fatalf("downstream CallTool() error = %v", err)
	}
	if mcpResult.IsError {
		t.Fatalf("downstream CallTool() returned IsError")
	}
	structured, ok := mcpResult.StructuredContent.(map[string]any)
	if !ok || structured["path"] != "mcp" {
		t.Fatalf("downstream result = %#v", mcpResult.StructuredContent)
	}

	if got := upstreamCalls.Load(); got != 2 {
		t.Fatalf("upstream call count = %d, want 2", got)
	}
}
