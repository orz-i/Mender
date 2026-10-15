package bootstrap

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	credentialapp "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func exampleTunnelPackage(t *testing.T) string {
	t.Helper()
	root := bootstrapRepositoryRoot(t)
	source := filepath.Join(root, "examples", "tunnel-plugin-example")
	path := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(path, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	binName := "mender-tunnel-example"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	command := exec.Command("go", "build", "-o", filepath.Join(path, "bin", binName), "./cmd/mender-tunnel-example") // #nosec G204 -- fixed local repository fixture built into TempDir.
	command.Dir = source
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build tunnel fixture: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(source, "mender-plugin.json")) // #nosec G304 -- static repository fixture.
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- destination is fixed manifest filename beneath test TempDir.
	if err := os.WriteFile(filepath.Join(path, "mender-plugin.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
func setupExternalTunnelFixture(t *testing.T) (*Runtime, installationdomain.Installation, string) {
	t.Helper()
	t.Setenv("MENDER_TUNNEL_PLUGIN_FIXTURE_SECRET", "runtime-fixture-secret")
	source := exampleTunnelPackage(t)
	root := t.TempDir()
	managed := filepath.Join(root, "plugins")
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(managed) })
	db := filepath.Join(root, "mender.db")
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{DatabasePath: db, ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("Runtime.Close: %v", err)
		}
	})
	installed, err := runtime.InstallTunnelPlugin(context.Background(), source)
	if err != nil {
		t.Fatalf("install external Tunnel plugin: %v", err)
	}
	return runtime, installed, managed
}
func fixtureTunnelCredential(t *testing.T, r *Runtime) string {
	t.Helper()
	item, err := r.Credentials.CreateEnvironment.Execute(context.Background(), credentialapp.CreateEnvironmentRequest{
		Namespace: "tunnel-fixture", Name: "secret", DisplayName: "Tunnel fixture credential",
		EnvironmentVariable: "MENDER_TUNNEL_PLUGIN_FIXTURE_SECRET",
	})
	if err != nil {
		t.Fatal(err)
	}
	return item.Reference().String()
}
func fixtureExternalTunnel(t *testing.T, r *Runtime, installed installationdomain.Installation,
	externalID string) (tunneldomain.Tunnel, profiledomain.Profile) {
	t.Helper()
	ctx := context.Background()
	rawRef := fixtureTunnelCredential(t, r)
	ref, err := credentialdomain.ParseReference(rawRef)
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := r.Tunnels.CreateExternal(ctx, CreateExternalTunnelRequest{
		DisplayName: "Example Transport", ExternalTunnelID: externalID,
		InstallationID: installed.ID(), RuntimeCredentialRef: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "external-tunnel", DisplayName: "External Tunnel", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = r.Profiles.SetTunnel.Execute(ctx, profileapp.SetTunnelRequest{ProfileSlug: profile.Slug(), TunnelID: tunnel.ID()})
	if err != nil {
		t.Fatal(err)
	}
	return tunnel, profile
}

func TestNativeTunnelTransportManagedMCPAndReferences(t *testing.T) {
	ctx := context.Background()
	r, installed, _ := setupExternalTunnelFixture(t)
	if installed.Extension().Kind != platformplugin.TunnelTransport {
		t.Fatalf("wrong installed kind %s", installed.Extension().Kind)
	}
	if r.externalProcesses.Running(installed.ID()) {
		t.Fatal("installation descriptor probe left process running")
	}
	available, err := r.ListAvailableProviderPlugins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range available {
		if candidate.InstallationID != nil && *candidate.InstallationID == installed.ID() {
			t.Fatal("Tunnel transport produced upstream Tools")
		}
	}
	if r.externalProcesses.Running(installed.ID()) {
		t.Fatal("catalogue read launched Tunnel plugin")
	}
	tunnel, profile := fixtureExternalTunnel(t, r, installed, "fixture-control-plane")
	if tunnel.Provider() != tunneldomain.ProviderExternal ||
		tunnel.Binding().InstallationID != installed.ID() {
		t.Fatalf("wrong exact Tunnel binding %+v", tunnel.Binding())
	}
	ref := tunnel.RuntimeCredentialRef()
	if err := r.Tunnels.Delete.Execute(ctx, tunnel.ID()); err == nil {
		t.Fatal("Profile-linked Tunnel was deleted")
	}
	if err := r.Credentials.Delete.Execute(ctx, ref); err == nil {
		t.Fatal("Tunnel Credential was deleted")
	}
	if _, err := r.SetTunnelPluginInstallationStatus(ctx, installed.ID(), installationdomain.StatusDisabled); err == nil {
		t.Fatal("referenced Tunnel transport disabled")
	}
	if err := r.RemoveTunnelPluginInstallation(ctx, installed.ID()); err == nil {
		t.Fatal("referenced Tunnel transport removed")
	}
	if err := r.pluginInstallations.SetStatus(ctx, installed.ID(), installationdomain.StatusDisabled); err == nil {
		t.Fatal("database bypass disabled referenced Tunnel package")
	}
	if err := r.pluginInstallations.Delete(ctx, installed.ID()); err == nil {
		t.Fatal("database bypass deleted referenced Tunnel package")
	}
	instance, err := startMCPHTTPServerTest(t, r, ctx, profile.Slug(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start managed loopback MCP Tunnel: %v", err)
	}
	if instance.TunnelID != tunnel.ID().String() || instance.TunnelStatus != "ready" ||
		instance.TunnelProvider != "external" || instance.TunnelExternalID != tunnel.ExternalTunnelID() {
		t.Fatalf("native Tunnel not attached to managed MCP: %+v", instance)
	}
	if !strings.HasPrefix(instance.Endpoint, "http://127.0.0.1:") {
		t.Fatalf("managed server bound non-loopback: %s", instance.Endpoint)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, instance.Endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode >= 500 {
		t.Fatalf("managed MCP server failed health: %d", response.StatusCode)
	}
	if err := r.StopMCPServer(ctx, instance.ServerID); err != nil {
		t.Fatal(err)
	}
	if len(r.ListMCPServers()) != 0 {
		t.Fatal("managed MCP server remained after stop")
	}
	restarted, err := startMCPHTTPServerTest(t, r, ctx, profile.Slug(), "127.0.0.1:0")
	if err != nil || restarted.TunnelStatus != "ready" {
		t.Fatalf("reconnected transport: %+v %v", restarted, err)
	}
	if err := r.StopMCPServer(ctx, restarted.ServerID); err != nil {
		t.Fatal(err)
	}
	profile, err = r.Profiles.SetTunnel.Execute(ctx, profileapp.SetTunnelRequest{ProfileSlug: profile.Slug()})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := profile.TunnelID(); present {
		t.Fatal("Profile Tunnel reference not cleared")
	}
	// Unbound Tunnel inventory is still an authoritative package reference.
	if _, err := r.SetTunnelPluginInstallationStatus(ctx, installed.ID(), installationdomain.StatusDisabled); err == nil {
		t.Fatal("unbound Tunnel allowed package disable")
	}
	if err := r.Tunnels.Delete.Execute(ctx, tunnel.ID()); err != nil {
		t.Fatal(err)
	}
	if err := r.Credentials.Delete.Execute(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveTunnelPluginInstallation(ctx, installed.ID()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeTunnelConnectFailureRollsBackMCPServer(t *testing.T) {
	ctx := context.Background()
	r, installed, _ := setupExternalTunnelFixture(t)
	_, profile := fixtureExternalTunnel(t, r, installed, "force-fail")
	_, err := startMCPHTTPServerTest(t, r, ctx, profile.Slug(), "127.0.0.1:0")
	if err == nil || !strings.Contains(err.Error(), "declined Tunnel Transport") {
		t.Fatalf("failed native Tunnel was reported running: %v", err)
	}
	if len(r.ListMCPServers()) != 0 {
		t.Fatal("managed local MCP listener left open after Tunnel Connect failure")
	}
	if r.externalProcesses.Running(installed.ID()) {
		t.Fatal("uncertain native Connect left executable running")
	}
}

func TestNativeTunnelHealthDegradedAndProcessCrash(t *testing.T) {
	ctx := context.Background()
	r, installed, _ := setupExternalTunnelFixture(t)
	_, profile := fixtureExternalTunnel(t, r, installed, "degraded")
	instance, err := startMCPHTTPServerTest(t, r, ctx, profile.Slug(), "127.0.0.1:0")
	if err != nil || instance.TunnelStatus != "degraded" {
		t.Fatalf("degraded status was advertised as ready: %+v %v", instance, err)
	}
	if err := r.externalProcesses.CloseInstallation(installed.ID()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	status := "degraded"
	for time.Now().Before(deadline) {
		servers := r.ListMCPServers()
		if len(servers) != 1 {
			t.Fatalf("MCP server vanished after transport crash: %+v", servers)
		}
		status = servers[0].TunnelStatus
		if status == "exited" {
			break
		}
		time.Sleep(60 * time.Millisecond)
	}
	if status != "exited" {
		t.Fatalf("native plugin process crash not observed: %s", status)
	}
	// Stopping a dead transport must still tear down the managed MCP listener.
	_ = r.StopMCPServer(ctx, instance.ServerID)
	if len(r.ListMCPServers()) != 0 {
		t.Fatal("crashed Tunnel left managed listener registered")
	}
}

func TestTunnelTransportRejectsUnsafeEndpointBeforeNativeLaunch(t *testing.T) {
	ctx := context.Background()
	r, installed, _ := setupExternalTunnelFixture(t)
	tunnel, _ := fixtureExternalTunnel(t, r, installed, "safe")
	// Explicit config validation is permitted during create, but an unsafe
	// managed endpoint must never restart a native executable.
	if err := r.externalProcesses.CloseInstallation(installed.ID()); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{
		"http://example.com:8000/mcp", "https://127.0.0.1:8000/mcp",
		"http://0.0.0.0:8000/mcp", "http://127.0.0.1:8000/mcp?secret=x",
		"http://127.0.0.1:8000/not-mcp", "file:///tmp/mcp",
	} {
		if err := tunnelapplication.ValidateManagedMCPEndpoint(endpoint); err == nil {
			t.Fatalf("unsafe MCP endpoint accepted by host %s", endpoint)
		}
		if _, _, err := r.Tunnels.Start.Execute(ctx, tunnel.ID(), endpoint); err == nil {
			t.Fatalf("unsafe Tunnel endpoint passed to transport %s", endpoint)
		}
		if r.externalProcesses.Running(installed.ID()) {
			t.Fatal("unsafe endpoint launched native Tunnel transport")
		}
	}
}

// Unbinding a Tunnel from its Profile is not enough to delete a resource
// while a managed MCP server is still actively exposing it.
func TestNativeTunnelDeleteBlockedDuringManagedMCPTeardown(t *testing.T) {
	ctx := context.Background()
	r, installed, _ := setupExternalTunnelFixture(t)
	tunnel, profile := fixtureExternalTunnel(t, r, installed, "active")
	instance, err := startMCPHTTPServerTest(t, r, ctx, profile.Slug(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Profiles.SetTunnel.Execute(ctx, profileapp.SetTunnelRequest{ProfileSlug: profile.Slug()}); err != nil {
		t.Fatal(err)
	}
	if err := r.Tunnels.Delete.Execute(ctx, tunnel.ID()); err == nil {
		t.Fatal("active Tunnel deleted after Profile unbind")
	}
	if err := r.StopMCPServer(ctx, instance.ServerID); err != nil {
		t.Fatal(err)
	}
	if err := r.Tunnels.Delete.Execute(ctx, tunnel.ID()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeTunnelPackageTamperingRollsBackBeforeConnect(t *testing.T) {
	ctx := context.Background()
	r, installed, managed := setupExternalTunnelFixture(t)
	_, profile := fixtureExternalTunnel(t, r, installed, "verify-tamper")
	exe := filepath.Join(managed, installed.ID().String(), "bin", "mender-tunnel-example")
	if err := os.Chmod(exe, 0o600); err != nil {
		t.Fatal(err)
	} // #nosec G302 -- test-owned managed plugin fixture.
	f, err := os.OpenFile(exe, os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304,G302 -- fixed test-owned plugin path.
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.WriteString("tampered-binary-bytes")
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("tamper fixture: %v %v", writeErr, closeErr)
	}
	if err := os.Chmod(exe, 0o500); err != nil {
		t.Fatal(err)
	} // #nosec G302 -- test executable restored to managed mode.
	// The config probe process may still be running; Preflight nevertheless
	// re-verifies the pinned immutable package bytes before Credential lookup.
	_, err = startMCPHTTPServerTest(t, r, ctx, profile.Slug(), "127.0.0.1:0")
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("tampered package allowed Tunnel Connect: %v", err)
	}
	if len(r.ListMCPServers()) != 0 {
		t.Fatal("failed start left managed MCP server")
	}
}

func TestNativeTunnelCreationRejectsOtherPluginKindAndDisabledDriver(t *testing.T) {
	ctx := context.Background()
	r, installed, _ := setupExternalTunnelFixture(t)
	sandboxSource := exampleSandboxPluginPackage(t)
	sandboxPlugin, err := r.InstallSandboxPlugin(ctx, sandboxSource)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := credentialdomain.ParseReference(fixtureTunnelCredential(t, r))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Tunnels.CreateExternal(ctx, CreateExternalTunnelRequest{
		DisplayName: "Wrong Type", ExternalTunnelID: "other", RuntimeCredentialRef: ref,
		InstallationID: sandboxPlugin.ID(),
	}); err == nil {
		t.Fatal("sandbox.backend package was accepted as tunnel.transport")
	}
	if _, err := r.SetTunnelPluginInstallationStatus(ctx, installed.ID(), installationdomain.StatusDisabled); err != nil {
		t.Fatalf("unused Tunnel plugin could not be disabled: %v", err)
	}
	id, _ := tunneldomain.ParseTunnelID("tun_0123456789abcdef0123456789abcdef")
	tunnel, err := tunneldomain.NewTunnel(id, "Disabled", tunneldomain.ProviderExternal,
		tunneldomain.Binding{PluginID: installed.PluginID(), ExtensionID: "main", InstallationID: installed.ID()},
		"disabled-tunnel", ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Tunnels.store.Create(ctx, tunnel); err == nil {
		t.Fatal("atomic SQLite insert accepted disabled Tunnel transport")
	}
	if _, err := r.Tunnels.CreateExternal(ctx, CreateExternalTunnelRequest{
		DisplayName: "Disabled", ExternalTunnelID: "disabled-tunnel", RuntimeCredentialRef: ref,
		InstallationID: installed.ID(),
	}); err == nil {
		t.Fatal("management created Tunnel on disabled plugin")
	}
	if len(r.ListMCPServers()) != 0 {
		t.Fatal("test started MCP listeners unexpectedly")
	}
}
