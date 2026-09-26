package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestManagedMCPHTTPServerLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "managed-server-upstream", Version: "1.0.0"},
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

	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	provider, err := runtime.RegisterMCPProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "upstream",
			DisplayName: "Upstream",
		},
		mcpconfigstore.Config{
			Transport: mcpconfigstore.TransportHTTP,
			Endpoint:  upstreamHTTP.URL,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SyncConfiguredMCPProvider(ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}

	search, err := runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{
			Query: "echo",
			Limit: 10,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(search) != 1 {
		t.Fatalf("search = %#v", search)
	}

	profile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "default",
			DisplayName: "Default",
			Mode:        profiledomain.ExposureDirect,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = runtime.Profiles.AddTool.Execute(
		ctx,
		profileapplication.AddToolRequest{
			ProfileSlug: profile.Slug(),
			ToolID:      search[0].ToolID,
			Direct:      true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	instance, err := runtime.StartMCPHTTPServer(
		ctx,
		profile.Slug(),
		"127.0.0.1:0",
	)
	if err != nil {
		t.Fatalf("StartMCPHTTPServer() error = %v", err)
	}
	if instance.Status != "running" ||
		instance.Transport != "streamable-http" ||
		!strings.HasSuffix(instance.Endpoint, "/mcp") {
		t.Fatalf("instance = %#v", instance)
	}
	if got := runtime.ListMCPServers(); len(got) != 1 ||
		got[0].ServerID != instance.ServerID {
		t.Fatalf("ListMCPServers() = %#v", got)
	}

	assertManagedServerExportsTool(t, instance.Endpoint, "upstream__echo")

	restarted, err := runtime.RestartMCPServer(ctx, instance.ServerID)
	if err != nil {
		t.Fatalf("RestartMCPServer() error = %v", err)
	}
	if restarted.ServerID == instance.ServerID {
		t.Fatalf("restart reused server id %q", restarted.ServerID)
	}
	assertManagedServerExportsTool(t, restarted.Endpoint, "upstream__echo")

	if err := runtime.StopMCPServer(ctx, restarted.ServerID); err != nil {
		t.Fatalf("StopMCPServer() error = %v", err)
	}
	if got := runtime.ListMCPServers(); len(got) != 0 {
		t.Fatalf("servers after stop = %#v", got)
	}
	if err := runtime.StopMCPServer(ctx, restarted.ServerID); !errors.Is(err, ErrMCPServerNotFound) {
		t.Fatalf("second StopMCPServer() error = %v", err)
	}
}

func TestManagedMCPHTTPServerRejectsNonLoopbackBind(t *testing.T) {
	t.Parallel()

	runtime, err := NewRuntime(context.Background(), RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	slug, err := profiledomain.NewSlug("default")
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.StartMCPHTTPServer(
		context.Background(),
		slug,
		"0.0.0.0:8765",
	)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("StartMCPHTTPServer() error = %v, want loopback rejection", err)
	}
}

func TestRuntimeCloseStopsManagedMCPServers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}

	profile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "empty",
			DisplayName: "Empty",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runtime.StartMCPHTTPServer(
		ctx,
		profile.Slug(),
		"127.0.0.1:0",
	); err != nil {
		t.Fatal(err)
	}
	if len(runtime.ListMCPServers()) != 1 {
		t.Fatal("expected one running server before Runtime.Close")
	}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Runtime.Close() error = %v", err)
	}
	if len(runtime.ListMCPServers()) != 0 {
		t.Fatal("managed servers remain after Runtime.Close")
	}
}

func assertManagedServerExportsTool(
	t *testing.T,
	endpoint string,
	expectedName string,
) {
	t.Helper()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "managed-server-test-client", Version: "1.0.0"},
		nil,
	)
	session, err := client.Connect(
		context.Background(),
		&mcp.StreamableClientTransport{
			Endpoint:             endpoint,
			DisableStandaloneSSE: true,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("client.Connect(%q) error = %v", endpoint, err)
	}
	defer func() { _ = session.Close() }()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != expectedName {
		t.Fatalf("tools = %#v, want %q", listed.Tools, expectedName)
	}
}
