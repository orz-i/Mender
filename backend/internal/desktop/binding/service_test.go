package binding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

	status := service.GetSystemStatus()
	if !status.Ready || status.Policy.Editable || status.Credentials.Editable {
		t.Fatalf("system status = %#v", status)
	}

	created, err := service.CreateHTTPProvider(
		"upstream",
		"Upstream",
		"",
		upstreamHTTP.URL,
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
		"",
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("UpdateProvider() error = %v", err)
	}
	if updated.Provider.Namespace != "upstream" ||
		updated.Provider.DisplayName != "Upstream edited" ||
		updated.Provider.Description != "Edited provider" {
		t.Fatalf("updated provider = %#v", updated.Provider)
	}

	search, err := service.SearchTools("echo", 10, "")
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
	profile, err = service.AddProfileTool("default", toolID, true)
	if err != nil {
		t.Fatalf("AddProfileTool() error = %v", err)
	}
	if len(profile.Tools) != 1 || !profile.Tools[0].Direct {
		t.Fatalf("profile tools = %#v", profile.Tools)
	}

	scoped, err := service.SearchTools("echo", 10, "default")
	if err != nil {
		t.Fatalf("profile SearchTools() error = %v", err)
	}
	if len(scoped.Tools) != 1 || scoped.Tools[0].ToolID != toolID {
		t.Fatalf("profile search tools = %#v", scoped.Tools)
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

	audit, err := service.ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(audit.Records) != 1 {
		t.Fatalf("audit records = %#v", audit.Records)
	}
	record := audit.Records[0]
	if record.Caller != "desktop" || record.ResultKind != "complete" {
		t.Fatalf("audit record = %#v", record)
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
