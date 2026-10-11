package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	credentialapp "github.com/orz-i/Mender/backend/internal/credential/application"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
)

func TestUnifiedPluginCatalogueManagementContract(t *testing.T) {
	t.Setenv(bootstrap.DockerSocketEnvironment, "")
	t.Setenv(bootstrap.DaytonaExperimentalEnvironment, "")
	ctx := context.Background()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	server := httptest.NewServer(New(r))
	defer server.Close()
	result := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/plugins", nil, http.StatusOK)
	items, ok := result["plugins"].([]any)
	if !ok || len(items) < 6 {
		t.Fatalf("catalogue missing builtin types: %+v", result)
	}
	kinds := map[string]bool{}
	sandboxKeys := map[string]int{}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("invalid catalogue entry")
		}
		kind, ok := item["kind"].(string)
		if !ok {
			t.Fatal("missing typed extension kind")
		}
		kinds[kind] = true
		if kind == "sandbox.backend" {
			key := item["key"].(string)
			sandboxKeys[key]++
			switch key {
			case "io.mender.sandbox.local-workspace@builtin":
				if item["sourceKind"] != "builtin" || item["enabled"] != true || item["status"] != "enabled" {
					t.Fatalf("Local Workspace not enabled: %+v", item)
				}
			case "io.mender.sandbox.docker@builtin", "io.mender.sandbox.daytona@builtin":
				if item["sourceKind"] != "builtin" || item["enabled"] != false ||
					item["status"] != "unavailable" || item["issue"] == nil {
					t.Fatalf("quarantined Sandbox unexpectedly enabled: %+v", item)
				}
			case "io.mender.sandbox.e2b@planned":
				if item["sourceKind"] != "planned" || item["status"] != "planned" ||
					item["enabled"] != false || item["installationId"] != nil || item["packageVersion"] != "" ||
					item["configVersion"] != nil || item["protocolVersion"] != nil {
					t.Fatalf("E2B roadmap item misrepresented as executable: %+v", item)
				}
			}
		}
	}
	for _, key := range []string{
		"io.mender.sandbox.local-workspace@builtin",
		"io.mender.sandbox.docker@builtin",
		"io.mender.sandbox.daytona@builtin",
		"io.mender.sandbox.e2b@planned",
	} {
		if sandboxKeys[key] != 1 {
			t.Fatalf("expected one canonical Sandbox entry %q, got %d", key, sandboxKeys[key])
		}
	}
	for _, kind := range []string{"provider.tools", "sandbox.backend", "tunnel.transport"} {
		if !kinds[kind] {
			t.Fatalf("missing extension kind %s", kind)
		}
	}
	// Planned software has no Create API. Do not interpret a Marketplace
	// preview as a provisionable executable Sandbox Backend.
	{
		plannedReq, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/sandboxes/e2b", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := server.Client().Do(plannedReq)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound && response.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("planned E2B unexpectedly has a provisioning endpoint: %d", response.StatusCode)
		}
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/plugin-installations/pinst_missing/disable", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/external", map[string]any{
		"name": "Denied", "installationId": "invalid", "config": map[string]any{},
	}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/tunnels/external", map[string]any{
		"displayName": "Denied", "installationId": "invalid", "externalTunnelId": "test", "runtimeCredentialRef": "credential://demo/key", // #nosec G101 -- non-secret test-only Credential Reference.
	}, http.StatusBadRequest)
	// No file-system install endpoint exists on the HTTP management plane.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/plugin-installations", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("public path installation unexpectedly exposed: %d", response.StatusCode)
	}
}

// Drive the HTTP surface through real verified Sandbox and Tunnel example
// installations. The fixture is local-only and never uses a user database,
// Docker daemon, cloud account, public tunnel, or actual Credential secret.
func TestUnifiedPluginCatalogueHTTPExternalResourceFlow(t *testing.T) {
	t.Setenv("MENDER_PHASE27E_TEST_TOKEN", strings.Repeat("q", 24))
	ctx := context.Background()
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	t.Cleanup(func() {
		_ = filepath.WalkDir(managed, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				// #nosec G122,G302 -- walk is confined to this test's TempDir; restore writable dirs for cleanup.
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(root, "mender.db"), ProviderPluginRoot: managed,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})

	buildSource := func(folder, binary string) string {
		t.Helper()
		repository, err := filepath.Abs("../../../..")
		if err != nil {
			t.Fatal(err)
		}
		fixture := filepath.Join(repository, "examples", folder)
		packageRoot := filepath.Join(t.TempDir(), folder)
		executable := filepath.Join(packageRoot, "bin", binary)
		if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("go", "build", "-o", executable, "./cmd/"+binary) // #nosec G204 -- two hardcoded checked-in fixture names and a test-owned output path.
		cmd.Dir = fixture
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", folder, err, output)
		}
		raw, err := os.ReadFile(filepath.Join(fixture, "mender-plugin.json")) // #nosec G304 -- fixed checked-in fixture manifest.
		if err != nil {
			t.Fatal(err)
		}
		// #nosec G703 -- packageRoot and basename are fixed test-owned paths.
		if err := os.WriteFile(filepath.Join(packageRoot, "mender-plugin.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return packageRoot
	}
	sb, err := r.InstallPlugin(ctx, buildSource("sandbox-plugin-example", "mender-sandbox-example"), platformplugin.SandboxBackend)
	if err != nil {
		t.Fatal(err)
	}
	tn, err := r.InstallPlugin(ctx, buildSource("tunnel-plugin-example", "mender-tunnel-example"), platformplugin.TunnelTransport)
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.Credentials.CreateEnvironment.Execute(ctx, credentialapp.CreateEnvironmentRequest{
		Namespace: "phasee", Name: "tunnel", DisplayName: "Phase E fixture",
		EnvironmentVariable: "MENDER_PHASE27E_TEST_TOKEN",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(r))
	defer server.Close()
	base := server.URL + "/api/v1"

	sandbox := requestJSON(t, server.Client(), http.MethodPost, base+"/sandboxes/external", map[string]any{
		"name": "HTTP fixture Sandbox", "installationId": sb.ID().String(), "config": map[string]any{},
	}, http.StatusCreated)
	if sandbox["backend"] != "external" || sandbox["installationId"] != sb.ID().String() ||
		sandbox["pluginId"] != sb.PluginID().String() || sandbox["state"] != "ready" {
		t.Fatalf("HTTP Sandbox extension binding or state drift: %+v", sandbox)
	}
	tunnel := requestJSON(t, server.Client(), http.MethodPost, base+"/tunnels/external", map[string]any{
		"displayName": "HTTP fixture Tunnel", "installationId": tn.ID().String(),
		"externalTunnelId": "fixture", "runtimeCredentialRef": c.Reference().String(),
	}, http.StatusCreated)
	if tunnel["provider"] != "external" || tunnel["installationId"] != tn.ID().String() ||
		tunnel["pluginId"] != tn.PluginID().String() ||
		tunnel["runtimeCredentialRef"] != c.Reference().String() {
		t.Fatalf("HTTP Tunnel extension binding drift: %+v", tunnel)
	}
	catalogue := requestJSON(t, server.Client(), http.MethodGet, base+"/plugins", nil, http.StatusOK)
	entries, ok := catalogue["plugins"].([]any)
	if !ok {
		t.Fatalf("invalid catalogue: %+v", catalogue)
	}
	usages := map[string]float64{}
	for _, entry := range entries {
		obj, ok := entry.(map[string]any)
		if !ok {
			t.Fatal("malformed plugin catalogue entry")
		}
		key, _ := obj["key"].(string)
		number, _ := obj["usageCount"].(float64)
		usages[key] = number
	}
	for _, item := range []installationdomain.Installation{sb, tn} {
		key := item.PluginID().String() + "@" + item.ID().String()
		if usages[key] != 1 {
			t.Fatalf("catalogue reference %s count=%v", key, usages[key])
		}
		requestJSON(t, server.Client(), http.MethodPost,
			base+"/plugin-installations/"+item.ID().String()+"/disable", nil, http.StatusConflict)
	}
	requestJSON(t, server.Client(), http.MethodPost, base+"/sandboxes/external", map[string]any{
		"name": "Wrong-kind sandbox", "installationId": tn.ID().String(), "config": map[string]any{},
	}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, base+"/tunnels/external", map[string]any{
		"displayName": "   ", "installationId": tn.ID().String(),
		"externalTunnelId": "fixture", "runtimeCredentialRef": c.Reference().String(),
	}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, base+"/tunnels/external", map[string]any{
		"displayName": "Wrong-kind Tunnel", "installationId": sb.ID().String(),
		"externalTunnelId": "fixture", "runtimeCredentialRef": c.Reference().String(),
	}, http.StatusConflict)
}
