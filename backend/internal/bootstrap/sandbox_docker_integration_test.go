package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"
	"time"

	mobyclient "github.com/moby/moby/client"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sand "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sanddocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

// The opt-in live gate proves the Desktop's fixed-path automatic discovery
// composes with the same Router-backed Docker engine, without starting any
// container. Full Workspace operations are tested separately.
func TestRuntimeDockerAutodiscoveryLive(t *testing.T) {
	socket := os.Getenv("MENDER_DOCKER_TEST_SOCKET")
	if socket == "" {
		t.Skip("requires explicit MENDER_DOCKER_TEST_SOCKET")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(sanddocker.KnownLocalEndpoints(home, goruntime.GOOS, os.Getuid()), socket) {
		t.Skip("test Docker socket is not a known local auto-discovery candidate")
	}
	t.Setenv(DockerSocketEnvironment, "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "docker-autodiscovery.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	if err := r.Sandboxes.StartDockerAutoDiscovery(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.Sandboxes.StartDockerAutoDiscovery(ctx); err != nil {
		t.Fatal(err)
	}
	for !r.Sandboxes.DockerAvailable() {
		select {
		case <-ctx.Done():
			t.Fatal("Docker automatic discovery did not connect to the verified local Engine")
		case <-time.After(100 * time.Millisecond):
		}
	}
	images, err := r.Sandboxes.ListDockerImages(ctx)
	if err != nil {
		t.Fatalf("read-only cached Docker inventory: %v", err)
	}
	if expected := os.Getenv("MENDER_DOCKER_TEST_IMAGE"); expected != "" {
		found := false
		for _, entry := range images {
			if entry.ImageDigest == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("pre-cached immutable image %s absent from verified inventory: %+v", expected, images)
		}
	}
	entries, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dockerCount := 0
	for _, entry := range entries {
		if entry.PluginID == "io.mender.sandbox.docker" {
			dockerCount++
			if entry.Status != "enabled" || !entry.Enabled || entry.Key != "io.mender.sandbox.docker@builtin" || entry.Issue != "" {
				t.Fatalf("auto-discovered Docker Marketplace entry invalid: %+v", entry)
			}
		}
	}
	if dockerCount != 1 {
		t.Fatalf("auto-discovered Docker entries: %d", dockerCount)
	}
	// Do not stop the user's Docker Desktop to simulate an outage.
	// Exercise the exact Runtime management/Router health projection
	// and recover by probing the still-running original daemon.
	select {
	case <-r.Sandboxes.dockerAutoDone:
		t.Fatal("Docker monitoring stopped after first connection")
	default:
	}
	controller := r.Sandboxes.dockerController()
	probe, err := controller.Probe(ctx)
	if err != nil || probe.EngineID == "" {
		t.Fatalf("Docker Engine identity not pinned: %+v %v", probe, err)
	}
	owner := r.OwnerPrincipal()
	resource, err := sand.NewSandbox(sand.Spec{
		ID: "sbx_0123456789abcdef0123456789abcdef", Name: "Health projection",
		Backend: sand.BackendDocker, Binding: sand.BuiltinBinding(sand.BackendDocker),
		Isolation: sand.IsolationContainer, OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference(),
	})
	if err != nil {
		t.Fatal(err)
	}
	r.Sandboxes.updateDockerHealth(controller, errors.New("simulated Engine outage"))
	if r.Sandboxes.DockerAvailable() || r.Sandboxes.BackendAvailable(resource) {
		t.Fatal("Docker Sandbox still available after observed Engine loss")
	}
	offline, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, plugin := range offline {
		if plugin.PluginID == "io.mender.sandbox.docker" &&
			(plugin.Status != "unavailable" || plugin.Enabled || plugin.Issue == "") {
			t.Fatalf("registered but disconnected Engine still enabled: %+v", plugin)
		}
	}
	if _, err := r.Sandboxes.ListDockerImages(ctx); !errors.Is(err, sand.ErrBackendUnavailable) {
		t.Fatalf("offline Engine still exposed immutable images: %v", err)
	}
	if !r.Sandboxes.refreshDockerHealth(ctx) || !r.Sandboxes.DockerAvailable() ||
		!r.Sandboxes.BackendAvailable(resource) {
		t.Fatal("same pinned Docker Engine was not recovered")
	}
	restored, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, plugin := range restored {
		if plugin.PluginID == "io.mender.sandbox.docker" &&
			(plugin.Status != "enabled" || !plugin.Enabled || plugin.Issue != "") {
			t.Fatalf("recovered Docker plugin did not return to enabled: %+v", plugin)
		}
	}
}

// This opt-in integration validates the REAL production runtime composition:
// configured Docker Engine, Policy, Sandbox router, stable Workspace Provider
// SourceKeys and the same read/search/edit/shell container filesystem.
func TestRuntimeDockerSandboxProviderEndToEnd(t *testing.T) {
	socket, image := os.Getenv("MENDER_DOCKER_TEST_SOCKET"), os.Getenv("MENDER_DOCKER_TEST_IMAGE")
	if socket == "" || image == "" {
		t.Skip("requires explicit MENDER_DOCKER_TEST_SOCKET and MENDER_DOCKER_TEST_IMAGE")
	}
	// The Engine must remain disconnected until a deliberate activation.
	t.Setenv(DockerSocketEnvironment, "")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "runtime-docker-integration.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	if runtime.Sandboxes.DockerAvailable() {
		t.Fatal("Docker silently connected without explicit opt-in")
	}
	if err := runtime.Sandboxes.ActivateDocker(ctx, socket); err != nil {
		t.Fatalf("activate trusted Docker Engine: %v", err)
	}
	if !runtime.Sandboxes.DockerAvailable() {
		t.Fatal("real Engine unavailable after activation")
	}
	if err := runtime.Sandboxes.ActivateDocker(ctx, socket); err != nil {
		t.Fatalf("repeated session activation: %v", err)
	}
	plugins, err := runtime.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var dockerCards int
	for _, entry := range plugins {
		if entry.PluginID == "io.mender.sandbox.docker" {
			dockerCards++
			if !entry.Enabled || entry.Status != "enabled" || entry.Issue != "" {
				t.Fatalf("activated Docker Marketplace card: %+v", entry)
			}
		}
	}
	if dockerCards != 1 {
		t.Fatalf("expected one Docker Marketplace card, got %d", dockerCards)
	}
	owner := runtime.OwnerPrincipal()
	caller, err := gov.NewContext(gov.OriginInternal, owner, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	template := sanddocker.Template{ImageDigest: image, UserID: 10001, GroupID: 10001, CPUmilli: 1000, MemoryMiB: 512, WorkspaceMiB: 64, PIDs: 64}
	item, err := runtime.Sandboxes.CreateDockerSandbox(ctx, CreateDockerSandboxRequest{Name: "Docker End-to-End", Template: template}, caller)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		// Cleanup ONLY this test's randomly generated uniquely labeled container.
		endpoint, err := sanddocker.ParseEngineEndpoint(socket)
		if err != nil {
			t.Error(err)
			return
		}
		cli, err := mobyclient.New(mobyclient.WithHost(endpoint.URL()))
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = cli.Close() }()
		for generation := 1; generation <= 2; generation++ {
			name := fmt.Sprintf("mender-sbx-%s-g%d", strings.TrimPrefix(item.ID().String(), "sbx_"), generation)
			inspected, err := cli.ContainerInspect(cleanup, name, mobyclient.ContainerInspectOptions{})
			if err == nil && inspected.Container.Config != nil && inspected.Container.Config.Labels["io.mender.sandbox.id"] == item.ID().String() {
				_, err = cli.ContainerRemove(cleanup, inspected.Container.ID, mobyclient.ContainerRemoveOptions{Force: true})
				if err != nil {
					t.Errorf("cleanup Docker test sandbox %s: %v", name, err)
				}
			}
		}
	}()
	raw, err := json.Marshal(workspaceplugin.Config{SandboxID: item.ID().String()})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := runtime.RegisterConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{Namespace: "docker-test", DisplayName: "Docker Workspace"}, workspaceplugin.ID, raw, caller)
	if err != nil {
		t.Fatal(err)
	}
	synced, err := runtime.SyncConfiguredProvider(ctx, provider.ID(), caller)
	if err != nil || synced.Total != 4 {
		t.Fatalf("Docker provider Sync %+v %v", synced, err)
	}
	tools, err := runtime.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]catalogdomain.ToolID{}
	for _, tool := range tools {
		ids[tool.SourceKey()] = tool.ID()
	}
	invoke := func(sourceKey, arguments string) map[string]any {
		t.Helper()
		toolID, ok := ids[sourceKey]
		if !ok {
			t.Fatalf("missing Tool SourceKey: %s", sourceKey)
		}
		response, err := runtime.CallConfiguredTool(ctx, toolID, []byte(arguments), caller)
		if err != nil || response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("%s: outcome %s, err %v", sourceKey, response.Outcome.Kind(), err)
		}
		var result map[string]any
		if err := json.Unmarshal(response.Outcome.Content().Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	invoke("workspace.edit", `{"path":"notes.txt","operation":"create","content":"hello Docker\n"}`)
	read := invoke("workspace.read", `{"path":"notes.txt"}`)
	if read["content"] != "hello Docker" {
		t.Fatalf("Docker read %+v", read)
	}
	match := invoke("workspace.search", `{"mode":"content","query":"Docker"}`)
	if len(match["matches"].([]any)) != 1 {
		t.Fatalf("Docker search %+v", match)
	}
	executed := invoke("workspace.shell", `{"command":"cat notes.txt; id -u; test ! -e /var/run/docker.sock; echo socket=$?"}`)
	if executed["exitCode"] != float64(0) || !strings.Contains(fmt.Sprint(executed["stdout"]), "hello Docker") || !strings.Contains(fmt.Sprint(executed["stdout"]), "10001") {
		t.Fatalf("Docker shell %+v", executed)
	}
	if _, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, caller); err == nil {
		t.Fatal("destroyed Sandbox with live Workspace Provider binding")
	}
	if err := runtime.providerConfigs.Delete(ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	stopped, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStop, caller)
	if err != nil || stopped.State != sand.StateStopped {
		t.Fatalf("Docker stopped state %+v %v", stopped, err)
	}
	started, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStart, caller)
	if err != nil || started.Generation != 2 || started.State != sand.StateReady {
		t.Fatalf("Docker start %+v %v", started, err)
	}
	destroyed, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, caller)
	if err != nil || destroyed.State != sand.StateDestroyed {
		t.Fatalf("Docker destroy %+v %v", destroyed, err)
	}
	events, err := runtime.Sandboxes.LifecycleEvents(ctx, item.ID(), 10)
	if err != nil || len(events) != 3 {
		t.Fatalf("Docker lifecycle audit %+v %v", events, err)
	}
}
