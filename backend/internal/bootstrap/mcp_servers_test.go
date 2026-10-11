package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
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
	if _, err := syncConfiguredProviderTest(t, runtime, ctx, provider.ID()); err != nil {
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

	instance, err := startMCPHTTPServerTest(t, runtime,
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

	assertManagedServerRejectsHost(t, instance.Endpoint, "attacker.example")
	assertManagedServerRejectsOrigin(
		t,
		instance.Endpoint,
		"https://attacker.example",
	)
	assertManagedServerExportsTool(t, instance.Endpoint, "upstream__echo")

	restarted, err := restartMCPServerTest(t, runtime, ctx, instance.ServerID)
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
	_, err = startMCPHTTPServerTest(t, runtime,
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

	if _, err := startMCPHTTPServerTest(t, runtime,
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

func TestManagedMCPHTTPServerSupervisesProfileTunnel(t *testing.T) {
	t.Setenv("MENDER_TUNNEL_RUNTIME_KEY", "runtime-secret")

	ctx := context.Background()
	runner := &fakeTunnelRunner{}
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:        filepath.Join(t.TempDir(), "mender.db"),
		TunnelProcessRunner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	credential, err := runtime.Credentials.CreateEnvironment.Execute(
		ctx,
		credentialapplication.CreateEnvironmentRequest{
			Namespace:           "openai",
			Name:                "tunnel-runtime",
			DisplayName:         "OpenAI Tunnel Runtime",
			EnvironmentVariable: "MENDER_TUNNEL_RUNTIME_KEY",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	tunnel, err := runtime.Tunnels.Create.Execute(
		ctx,
		tunnelapplication.CreateOpenAITunnelRequest{
			DisplayName:          "OpenAI Work",
			ExternalTunnelID:     "tunnel_external_work",
			RuntimeCredentialRef: credential.Reference(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "tunneled",
			DisplayName: "Tunneled",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = runtime.Profiles.SetTunnel.Execute(
		ctx,
		profileapplication.SetTunnelRequest{
			ProfileSlug: profile.Slug(),
			TunnelID:    tunnel.ID(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := runtime.Tunnels.Delete.Execute(ctx, tunnel.ID()); !errors.Is(
		err,
		tunnelapplication.ErrTunnelInUse,
	) {
		t.Fatalf("Delete in-use Tunnel error = %v", err)
	}
	if err := runtime.Credentials.Delete.Execute(
		ctx,
		credential.Reference(),
	); !errors.Is(err, credentialapplication.ErrCredentialInUse) {
		t.Fatalf("Delete in-use Tunnel Credential error = %v", err)
	}

	instance, err := startMCPHTTPServerTest(t, runtime,
		ctx,
		profile.Slug(),
		"127.0.0.1:0",
	)
	if err != nil {
		t.Fatalf("StartMCPHTTPServer() error = %v", err)
	}
	if instance.TunnelID != tunnel.ID().String() ||
		instance.TunnelDisplayName != tunnel.DisplayName() ||
		instance.TunnelProvider != string(tunneldomain.ProviderOpenAI) ||
		instance.TunnelExternalID != tunnel.ExternalTunnelID() ||
		instance.TunnelStatus != "starting" {
		t.Fatalf("tunneled instance = %#v", instance)
	}
	first := runner.processAt(t, 0)
	if first.endpoint != instance.Endpoint || first.secret != "runtime-secret" {
		t.Fatalf("runner start = endpoint %q secret %q", first.endpoint, first.secret)
	}

	restarted, err := restartMCPServerTest(t, runtime, ctx, instance.ServerID)
	if err != nil {
		t.Fatalf("RestartMCPServer() error = %v", err)
	}
	if !first.stopped {
		t.Fatal("restart did not stop original tunnel process")
	}
	if restarted.TunnelStatus != "starting" {
		t.Fatalf("restarted tunnel status = %q", restarted.TunnelStatus)
	}
	second := runner.processAt(t, 1)
	if err := runtime.StopMCPServer(ctx, restarted.ServerID); err != nil {
		t.Fatal(err)
	}
	if !second.stopped {
		t.Fatal("stop did not stop restarted tunnel process")
	}

	profile, err = runtime.Profiles.SetTunnel.Execute(
		ctx,
		profileapplication.SetTunnelRequest{ProfileSlug: profile.Slug()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := profile.TunnelID(); ok {
		t.Fatal("Profile TunnelID remains after clear")
	}
	if err := runtime.Tunnels.Delete.Execute(ctx, tunnel.ID()); err != nil {
		t.Fatalf("Delete Tunnel after clear error = %v", err)
	}
	if err := runtime.Credentials.Delete.Execute(ctx, credential.Reference()); err != nil {
		t.Fatalf("Delete Tunnel Credential after Tunnel removal error = %v", err)
	}
}

func TestManagedMCPHTTPServerRollsBackWhenTunnelStartFails(t *testing.T) {
	t.Setenv("MENDER_TUNNEL_RUNTIME_KEY", "runtime-secret")

	ctx := context.Background()
	runner := &fakeTunnelRunner{startErr: errors.New("fake tunnel start failure")}
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:        filepath.Join(t.TempDir(), "mender.db"),
		TunnelProcessRunner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	credential, err := runtime.Credentials.CreateEnvironment.Execute(
		ctx,
		credentialapplication.CreateEnvironmentRequest{
			Namespace:           "openai",
			Name:                "failed-tunnel",
			DisplayName:         "Failed Tunnel Runtime",
			EnvironmentVariable: "MENDER_TUNNEL_RUNTIME_KEY",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := runtime.Tunnels.Create.Execute(
		ctx,
		tunnelapplication.CreateOpenAITunnelRequest{
			DisplayName:          "Failing Tunnel",
			ExternalTunnelID:     "tunnel_failure",
			RuntimeCredentialRef: credential.Reference(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "failing-tunnel",
			DisplayName: "Failing Tunnel",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Profiles.SetTunnel.Execute(
		ctx,
		profileapplication.SetTunnelRequest{
			ProfileSlug: profile.Slug(),
			TunnelID:    tunnel.ID(),
		},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := startMCPHTTPServerTest(t, runtime,
		ctx,
		profile.Slug(),
		"127.0.0.1:0",
	); err == nil || !strings.Contains(err.Error(), "fake tunnel start failure") {
		t.Fatalf("StartMCPHTTPServer() error = %v", err)
	}
	if servers := runtime.ListMCPServers(); len(servers) != 0 {
		t.Fatalf("servers after failed tunnel start = %#v", servers)
	}
}

type fakeTunnelRunner struct {
	mu        sync.Mutex
	processes []*fakeTunnelProcess
	startErr  error
}

func (r *fakeTunnelRunner) Preflight(_ context.Context, tunnel tunneldomain.Tunnel) error {
	return tunnel.Binding().ValidateProvider(tunneldomain.ProviderOpenAI)
}

func (r *fakeTunnelRunner) Start(
	_ context.Context,
	_ tunneldomain.Tunnel,
	endpoint string,
	apiKey []byte,
) (tunnelapplication.Process, error) {
	if r.startErr != nil {
		return nil, r.startErr
	}
	process := &fakeTunnelProcess{
		endpoint: endpoint,
		secret:   string(apiKey),
		done:     make(chan error),
	}
	r.mu.Lock()
	r.processes = append(r.processes, process)
	r.mu.Unlock()
	return process, nil
}

func (r *fakeTunnelRunner) processAt(t *testing.T, index int) *fakeTunnelProcess {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if index >= len(r.processes) {
		t.Fatalf("process index %d missing from %#v", index, r.processes)
	}
	return r.processes[index]
}

type fakeTunnelProcess struct {
	mu       sync.Mutex
	endpoint string
	secret   string
	stopped  bool
	done     chan error
	once     sync.Once
}

func (p *fakeTunnelProcess) Stop(context.Context) error {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *fakeTunnelProcess) Done() <-chan error {
	return p.done
}

func assertManagedServerRejectsHost(
	t *testing.T,
	endpoint string,
	host string,
) {
	t.Helper()

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = host
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf(
			"Host %q status = %d, want %d",
			host,
			response.StatusCode,
			http.StatusForbidden,
		)
	}
}

func assertManagedServerRejectsOrigin(
	t *testing.T,
	endpoint string,
	origin string,
) {
	t.Helper()

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", origin)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf(
			"Origin %q status = %d, want %d",
			origin,
			response.StatusCode,
			http.StatusForbidden,
		)
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
