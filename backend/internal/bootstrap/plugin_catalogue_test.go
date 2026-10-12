package bootstrap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestCanonicalPluginCatalogueAllTypesInertAndStable(t *testing.T) {
	t.Setenv(DockerSocketEnvironment, "")
	t.Setenv(DaytonaExperimentalEnvironment, "")
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	first, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) || len(first) < 6 {
		t.Fatalf("missing typed built-ins: %+v", first)
	}
	seen := map[platformplugin.Kind]int{}
	keys := map[string]bool{}
	for _, plugin := range first {
		seen[plugin.Kind]++
		if keys[plugin.Key] {
			t.Fatalf("duplicate catalogue key %s", plugin.Key)
		}
		keys[plugin.Key] = true
		if plugin.InstalledAt != "" || plugin.InstallationID != "" || plugin.UsageCount != 0 {
			t.Fatalf("untrusted process or fake installation on catalogue entry: %+v", plugin)
		}
		switch plugin.PluginID {
		case "io.mender.sandbox.docker", "io.mender.sandbox.daytona":
			if plugin.SourceKind != "builtin" || plugin.Enabled || plugin.Status != "unavailable" ||
				plugin.Issue == "" || len(plugin.Capabilities) != 4 {
				t.Fatalf("quarantined Sandbox must remain discoverable but unavailable: %+v", plugin)
			}
		case "io.mender.sandbox.e2b":
			if plugin.SourceKind != "planned" || plugin.Status != "planned" || plugin.Enabled ||
				plugin.Key != "io.mender.sandbox.e2b@planned" || len(plugin.Capabilities) != 0 ||
				plugin.PackageVersion != "" || plugin.ProtocolVersion != 0 || plugin.ConfigVersion != 0 {
				t.Fatalf("E2B roadmap preview masquerades as an installed plugin: %+v", plugin)
			}
		case "io.mender.agent.codex", "io.mender.agent.grok":
			if plugin.SourceKind != "builtin" || plugin.Kind != platformplugin.AgentBackend || len(plugin.Capabilities) != 3 ||
				(plugin.Status != "enabled" && plugin.Status != "unavailable") ||
				(plugin.Status == "unavailable" && (plugin.Enabled || plugin.Issue == "")) {
				t.Fatalf("Agent Backend status or trust incorrectly reported: %+v", plugin)
			}
		case "io.mender.sandbox.ssh-remote":
			if plugin.Status != "enabled" || !plugin.Enabled || plugin.SourceKind != "builtin" || len(plugin.Capabilities) != 4 ||
				plugin.Key != "io.mender.sandbox.ssh-remote@builtin" || plugin.Issue != "" {
				t.Fatalf("trusted SSH remote plugin unavailable: %+v", plugin)
			}
		default:
			if plugin.SourceKind != "builtin" || !plugin.Enabled || plugin.Status != "enabled" {
				t.Fatalf("registered builtin unexpectedly unavailable: %+v", plugin)
			}
		}
	}
	for _, kind := range []platformplugin.Kind{platformplugin.ProviderTools, platformplugin.SandboxBackend, platformplugin.TunnelTransport, platformplugin.AgentBackend} {
		if seen[kind] == 0 {
			t.Fatalf("catalogue lacks extension kind %s", kind)
		}
	}
	for _, key := range []string{
		"io.mender.sandbox.local-workspace@builtin",
		"io.mender.sandbox.docker@builtin",
		"io.mender.sandbox.daytona@builtin",
		"io.mender.sandbox.ssh-remote@builtin",
		"io.mender.sandbox.e2b@planned",
	} {
		if !keys[key] {
			t.Fatalf("Sandbox option %q missing from canonical Marketplace", key)
		}
	}
	if len(r.ListMCPServers()) != 0 {
		t.Fatal("catalogue started MCP server")
	}
	if err := r.Sandboxes.ActivateDocker(ctx, "tcp://127.0.0.1:2375"); err == nil {
		t.Fatal("untrusted Docker TCP endpoint activated")
	}
	if r.Sandboxes.DockerAvailable() {
		t.Fatal("rejected activation registered a Docker backend")
	}
	if r.Sandboxes.DaytonaConfigured() {
		t.Fatal("catalogue discovery unexpectedly enabled experimental Daytona")
	}
	if r.Sandboxes.backendPlugins.Available(sandboxdomain.BackendE2B, sandboxdomain.BuiltinBinding(sandboxdomain.BackendE2B)) {
		t.Fatal("catalogue preview registered unimplemented E2B")
	}
	after, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, plugin := range after {
		if plugin.PluginID == "io.mender.sandbox.docker" && (plugin.Enabled || plugin.Status != "unavailable") {
			t.Fatalf("failed activation changed Docker catalogue availability: %+v", plugin)
		}
	}
}

func TestCanonicalCatalogueExperimentalDaytonaOptInOnlyChangesItsAvailability(t *testing.T) {
	t.Setenv(DockerSocketEnvironment, "")
	t.Setenv(DaytonaExperimentalEnvironment, "enabled")
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	entries, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var daytona, e2b int
	for _, entry := range entries {
		switch entry.PluginID {
		case "io.mender.sandbox.daytona":
			daytona++
			if entry.Key != "io.mender.sandbox.daytona@builtin" || entry.Status != "enabled" ||
				!entry.Enabled || entry.Issue != "" || entry.SourceKind != "builtin" {
				t.Fatalf("opted-in Daytona did not become configurable: %+v", entry)
			}
		case "io.mender.sandbox.e2b":
			e2b++
			if entry.Status != "planned" || entry.Enabled || entry.SourceKind != "planned" {
				t.Fatalf("Daytona opt-in activated unrelated E2B preview: %+v", entry)
			}
		}
	}
	if daytona != 1 || e2b != 1 {
		t.Fatalf("incorrect Sandbox Marketplace cardinality: Daytona=%d E2B=%d", daytona, e2b)
	}
}

// The catalogue must include every installed extension kind and report
// installation-level resource usage without resolving Credentials or launching
// plugin executables. Every package is a local test-built, no-cloud fixture.
func TestCanonicalCatalogueInstalledThreeKindsAndUsage(t *testing.T) {
	t.Setenv("MENDER_CATALOGUE_TEST_SECRET", "local-fixture-only")
	ctx := context.Background()
	root := t.TempDir()
	managed := filepath.Join(root, "plugins")
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(managed) })
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(root, "mender.db"), ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})

	providerRoot := filepath.Join(t.TempDir(), "provider")
	binary := filepath.Join(providerRoot, "bin", examplePluginBinaryName())
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(bootstrapRepositoryRoot(t), "examples", "provider-plugin-example")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/mender-provider-example") // #nosec G204 -- trusted checked-in fixture with TempDir output.
	cmd.Dir = source
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Provider fixture: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(source, "mender-plugin.json")) // #nosec G304 -- fixed checked-in manifest fixture.
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- destination fixed beneath testing TempDir.
	if err := os.WriteFile(filepath.Join(providerRoot, "mender-plugin.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	provider, err := runtime.InstallPlugin(ctx, providerRoot, platformplugin.ProviderTools)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := runtime.InstallPlugin(ctx, exampleSandboxPluginPackage(t), platformplugin.SandboxBackend)
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := runtime.InstallPlugin(ctx, exampleTunnelPackage(t), platformplugin.TunnelTransport)
	if err != nil {
		t.Fatal(err)
	}

	for _, installed := range []installationdomain.Installation{provider, sandbox, tunnel} {
		if runtime.externalProcesses.Running(installed.ID()) {
			t.Fatalf("installed %s left plugin running", installed.ID())
		}
	}
	catalogue, err := runtime.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, installed := range []installationdomain.Installation{provider, sandbox, tunnel} {
		key := installed.PluginID().String() + "@" + installed.ID().String()
		found := false
		for _, entry := range catalogue {
			if entry.Key != key {
				continue
			}
			found = true
			if entry.Kind != installed.Extension().Kind || entry.UsageCount != 0 || entry.InstallationID != installed.ID().String() ||
				!entry.Enabled || entry.SourceKind != "local" || entry.ProtocolVersion != 1 {
				t.Fatalf("mismatched typed catalogue entry: %+v", entry)
			}
		}
		if !found {
			t.Fatalf("installed extension %s not found in unified catalogue", key)
		}
		if runtime.externalProcesses.Running(installed.ID()) {
			t.Fatalf("read-only catalogue launched %s", installed.ID())
		}
	}
	// Persist real governed fixture resources for two unrelated domains.
	item, err := runtime.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(sandbox.ID(), `{}`), ownerContext(t, runtime))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := runtime.Credentials.CreateEnvironment.Execute(ctx, credentialapplication.CreateEnvironmentRequest{
		Namespace: "catalogue", Name: "test", DisplayName: "Fixture", EnvironmentVariable: "MENDER_CATALOGUE_TEST_SECRET",
	})
	if err != nil {
		t.Fatal(err)
	}
	remoteTunnel, err := runtime.Tunnels.CreateExternal(ctx, CreateExternalTunnelRequest{
		DisplayName: "Example Tunnel", InstallationID: tunnel.ID(), ExternalTunnelID: "test",
		RuntimeCredentialRef: ref.Reference(),
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := runtime.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	usages := map[string]int{}
	for _, entry := range updated {
		usages[entry.Key] = entry.UsageCount
	}
	for _, installed := range []installationdomain.Installation{sandbox, tunnel} {
		key := installed.PluginID().String() + "@" + installed.ID().String()
		if usages[key] != 1 {
			t.Fatalf("persisted resource usage for %s = %d", key, usages[key])
		}
		if _, err := runtime.SetPluginInstallationStatus(ctx, installed.ID(), installationdomain.StatusDisabled); err == nil {
			t.Fatalf("in-use plugin %s disabled through generic management", installed.ID())
		}
	}
	if got, err := runtime.Sandboxes.Get(ctx, item.ID()); err != nil || got.ID() != item.ID() {
		t.Fatalf("Sandbox lost after catalogue read: %v", err)
	}
	if got, err := runtime.Tunnels.Get.Execute(ctx, remoteTunnel.ID()); err != nil || got.ID() != remoteTunnel.ID() {
		t.Fatalf("Tunnel lost after catalogue read: %v", err)
	}
	// An unused Provider package can change status independently of the
	// referenced Sandbox/Tunnel packages; no provider kind fallback.
	disabled, err := runtime.SetPluginInstallationStatus(ctx, provider.ID(), installationdomain.StatusDisabled)
	if err != nil || disabled.Status() != installationdomain.StatusDisabled {
		t.Fatalf("disable unused Provider extension: %+v %v", disabled, err)
	}
}
