package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	localworkspace "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/localworkspace"
)

type fixtureRouter struct {
	item               sandboxdomain.Sandbox
	run                sandboxdomain.Run
	backend            *localworkspace.Backend
	resolves, acquires int
	rejected           bool
}

func (r *fixtureRouter) Resolve(_ context.Context, id sandboxdomain.ID, capability sandboxdomain.Capability) (sandboxapp.Placement, error) {
	r.resolves++
	if id != r.item.ID() {
		return sandboxapp.Placement{}, sandboxdomain.ErrNotFound
	}
	if !r.backend.Capabilities().Supports(capability) {
		return sandboxapp.Placement{}, sandboxdomain.ErrCapabilityUnavailable
	}
	return sandboxapp.Placement{Sandbox: r.item, Run: r.run, Required: capability}, nil
}
func (r *fixtureRouter) Acquire(ctx context.Context, caller gov.Context, placement sandboxapp.Placement) (*sandboxapp.Lease, error) {
	r.acquires++
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if r.rejected {
		return nil, sandboxdomain.ErrNotAuthorized
	}
	session, err := r.backend.Open(ctx, placement.Sandbox, placement.Run)
	if err != nil {
		return nil, err
	}
	return sandboxapp.NewLease(r.item.ID(), r.run.Generation, session, session.Close), nil
}

type hostLauncher struct{}

func (hostLauncher) CheckIsolation(mode sandboxdomain.IsolationClass) error {
	if mode != sandboxdomain.IsolationHost {
		return sandboxdomain.ErrBackendUnavailable
	}
	return nil
}
func (hostLauncher) Command(ctx context.Context, _ sandboxdomain.IsolationClass, _, workingDirectory, command string, env []string) (*exec.Cmd, error) {
	// #nosec G204 -- test-only shell for bounded Sandbox integration tests.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = workingDirectory
	cmd.Env = env
	return cmd, nil
}
func openWorkspaceTestInstance(t *testing.T, root string) (*Instance, *fixtureRouter) {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: sandboxdomain.ID(testSandboxID), Name: "Workspace", Backend: sandboxdomain.BackendLocalWorkspace, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace), Isolation: sandboxdomain.IsolationHost, LocalRoot: canonical, OwnerKind: "local_user", OwnerRef: "unit"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(item.ID(), 1, sandboxdomain.StateReady, "local")
	if err != nil {
		t.Fatal(err)
	}
	backend, err := localworkspace.New(hostLauncher{})
	if err != nil {
		t.Fatal(err)
	}
	router := &fixtureRouter{item: item, run: run, backend: backend}
	raw, err := json.Marshal(Config{SandboxID: testSandboxID})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := New(router).Open(context.Background(), providerplugin.OpenRequest{ConfigVersion: ConfigVersion, Config: raw})
	if err != nil {
		t.Fatal(err)
	}
	instance := opened.(*Instance)
	t.Cleanup(func() { _ = instance.Close() })
	return instance, router
}
func workspaceTestTool(t *testing.T, key string) catalogdomain.Tool {
	t.Helper()
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000051")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000051")
	schema, _ := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	tool, err := catalogdomain.NewTool(toolID, providerID, catalogdomain.ToolSpec{SourceKey: key, Name: key, InputSchema: schema}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tool
}
func fixtureCaller(t *testing.T) gov.Context {
	t.Helper()
	p, err := gov.NewPrincipal(gov.PrincipalLocalUser, "unit")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginInternal, p, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return caller
}
func runWorkspace(t *testing.T, instance *Instance, key string, args any) (invocationdomain.Outcome, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	val, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	return instance.Execute(context.Background(), invocationapp.ExecutionRequest{Tool: workspaceTestTool(t, key), Arguments: val, Context: fixtureCaller(t)}, nil, nil)
}
func TestWorkspacePluginV2StaticDiscoverAndGovernedExecution(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	instance, router := openWorkspaceTestInstance(t, root)
	tools, err := instance.DiscoverTools(context.Background(), catalogdomain.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(tools))
	for _, item := range tools {
		keys = append(keys, item.SourceKey)
	}
	if !slices.Equal(keys, []string{sourceKeyRead, sourceKeySearch, sourceKeyEdit, sourceKeyShell}) {
		t.Fatalf("keys %v", keys)
	}
	if router.resolves != 0 || router.acquires != 0 {
		t.Fatal("discovery acquired Sandbox")
	}
	for _, item := range []struct {
		key   string
		input map[string]any
	}{
		{sourceKeyRead, map[string]any{"path": "notes.txt"}},
		{sourceKeySearch, map[string]any{"mode": "content", "query": "alpha"}},
		{sourceKeyEdit, map[string]any{"path": "draft.txt", "operation": "create", "content": "hello"}},
		{sourceKeyShell, map[string]any{"command": "pwd"}},
	} {
		out, err := runWorkspace(t, instance, item.key, item.input)
		if err != nil || out.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("%s failed: %v %s", item.key, err, out.Kind())
		}
	}
	if router.resolves != 4 || router.acquires != 4 {
		t.Fatalf("not routed %+v", router)
	}
}
func TestDeniedSandboxAcquisitionNeverExecutesTool(t *testing.T) {
	root := t.TempDir()
	instance, router := openWorkspaceTestInstance(t, root)
	router.rejected = true
	result, err := runWorkspace(t, instance, sourceKeyEdit, map[string]any{"path": "blocked.txt", "operation": "create", "content": "secret"})
	if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("not denied: %v (outcome %s)", err, result.Kind())
	}
	if _, err := os.Stat(filepath.Join(root, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied Tool modified filesystem: %v", err)
	}
}
func TestWorkspaceV1OpenFailsClosed(t *testing.T) {
	_, router := openWorkspaceTestInstance(t, t.TempDir())
	legacy := json.RawMessage(`{"root":"/tmp"}`)
	if _, err := New(router).Open(context.Background(), providerplugin.OpenRequest{ConfigVersion: 1, Config: legacy}); err == nil {
		t.Fatal("v1 Plugin.Open accepted")
	}
	if _, err := New(router).Open(context.Background(), providerplugin.OpenRequest{ConfigVersion: 2, Config: legacy}); err == nil {
		t.Fatal("v1 JSON accepted")
	}
}

func TestWorkspaceConfigLookupRejectsDanglingSandboxWithoutBackendOpen(t *testing.T) {
	instance, router := openWorkspaceTestInstance(t, t.TempDir())
	_ = instance
	valid, err := json.Marshal(Config{SandboxID: testSandboxID})
	if err != nil {
		t.Fatal(err)
	}
	if err := New(router).ValidateConfig(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	if router.resolves != 1 || router.acquires != 0 {
		t.Fatalf("config check must be metadata-only: resolves=%d acquires=%d", router.resolves, router.acquires)
	}
	unknown, err := json.Marshal(Config{SandboxID: "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if err := New(router).ValidateConfig(context.Background(), unknown); !errors.Is(err, sandboxdomain.ErrNotFound) {
		t.Fatalf("dangling Sandbox reference accepted: %v", err)
	}
	if router.acquires != 0 {
		t.Fatal("validation acquired a Sandbox")
	}
	if err := New(nil).ValidateConfig(context.Background(), valid); err == nil {
		t.Fatal("missing execution router accepted")
	}
}
