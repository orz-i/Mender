package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	policyapp "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func exampleSandboxPluginPackage(t *testing.T) string {
	t.Helper()
	root := bootstrapRepositoryRoot(t)
	source := filepath.Join(root, "examples", "sandbox-plugin-example")
	pkg := filepath.Join(t.TempDir(), "sandbox-plugin")
	if err := os.MkdirAll(filepath.Join(pkg, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	binName := "mender-sandbox-example"
	if goruntime.GOOS == "windows" {
		binName += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(pkg, "bin", binName), "./cmd/mender-sandbox-example") // #nosec G204 -- fixed checked-in fixture in a temporary path.
	cmd.Dir = source
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build sandbox example: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(source, "mender-plugin.json")) // #nosec G304 -- checked-in test fixture.
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- package directory and manifest basename are fixed local test fixtures under TempDir.
	if err := os.WriteFile(filepath.Join(pkg, "mender-plugin.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return pkg
}
func ownerContext(t *testing.T, r *Runtime) governancedomain.Context {
	t.Helper()
	caller, err := governancedomain.NewContext(governancedomain.OriginCLI, r.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return caller
}
func fixtureSandboxRequest(inst platformplugin.InstallationID, config string) CreateExternalSandboxRequest {
	return CreateExternalSandboxRequest{Name: "Example backend", InstallationID: inst,
		Config: sandboxsqlite.ExternalConfig{ConfigVersion: 1, JSON: json.RawMessage(config)}}
}

func TestExternalSandboxPluginGovernedLifecycleAndExecution(t *testing.T) {
	ctx := context.Background()
	pkg := exampleSandboxPluginPackage(t)
	folder := t.TempDir()
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(filepath.Join(folder, "managed")) })
	db := filepath.Join(folder, "mender.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db, ProviderPluginRoot: filepath.Join(folder, "managed"), DefaultTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	installation, err := runtime.InstallSandboxPlugin(ctx, pkg)
	if err != nil {
		t.Fatalf("install sandbox plugin: %v", err)
	}
	if installation.Extension().Kind != platformplugin.SandboxBackend ||
		!runtime.Sandboxes.backendPlugins.Available(sandboxdomain.BackendExternal,
			sandboxdomain.BackendBinding{PluginID: installation.PluginID(), ExtensionID: "main", InstallationID: installation.ID()}) {
		t.Fatalf("sandbox plugin not registered: %+v", installation)
	}
	if runtime.externalProcesses.Running(installation.ID()) {
		t.Fatal("install probe left native plugin process running")
	}
	catalog, err := runtime.ListAvailableProviderPlugins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog {
		if entry.InstallationID != nil && *entry.InstallationID == installation.ID() {
			t.Fatal("sandbox plugin masqueraded as tool Provider")
		}
	}
	// A service cannot provision the owner sandbox even when it knows the installation.
	servicePrincipal, err := governancedomain.NewPrincipal(governancedomain.PrincipalService, "profile:unauthorized")
	if err != nil {
		t.Fatal(err)
	}
	serviceCtx, err := governancedomain.NewContext(governancedomain.OriginMCP, servicePrincipal, governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(installation.ID(), `{}`), serviceCtx); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("untrusted service provisioned Sandbox: %v", err)
	}
	if runtime.externalProcesses.Running(installation.ID()) {
		t.Fatal("denied provision started plugin")
	}
	caller := ownerContext(t, runtime)
	item, err := runtime.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(installation.ID(), `{"message":"example"}`), caller)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if item.Backend() != sandboxdomain.BackendExternal || item.Spec().Binding.InstallationID != installation.ID() {
		t.Fatalf("missing exact plugin pin: %+v", item.Spec())
	}
	run, err := runtime.Sandboxes.GetRun(ctx, item.ID())
	if err != nil || run.State != sandboxdomain.StateReady || run.Generation != 1 {
		t.Fatalf("provisioned run %+v %v", run, err)
	}
	// A disabled/missing package cannot replace a live Sandbox.
	if _, err := runtime.SetSandboxPluginInstallationStatus(ctx, installation.ID(), installationdomain.StatusDisabled); err == nil {
		t.Fatal("disabled referenced Sandbox plugin")
	}
	if err := runtime.RemoveSandboxPluginInstallation(ctx, installation.ID()); err == nil {
		t.Fatal("removed referenced Sandbox plugin")
	}
	place, err := runtime.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := runtime.Sandboxes.router.Acquire(ctx, caller, place)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	ws, ok := lease.Session.(sandboxapp.WorkspaceSession)
	if !ok {
		t.Fatal("external session has no workspace execution")
	}
	args, err := invocationdomain.NewJSONValue([]byte(`{"path":"README.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := ws.ExecuteWorkspace(ctx, sandboxdomain.CapabilityRead, args)
	if err != nil || outcome.Kind() != invocationdomain.OutcomeKindComplete ||
		!strings.Contains(string(outcome.Content().Bytes()), "Example Sandbox") {
		t.Fatalf("external workspace execution %+v %v", outcome, err)
	}
	if _, err := ws.ExecuteWorkspace(ctx, sandboxdomain.CapabilityShell, args); !errors.Is(err, sandboxdomain.ErrCapabilityUnavailable) {
		t.Fatalf("capability fallback: %v", err)
	}
	if _, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxDestroy, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("destroy ignored active lease: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	stopped, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStop, caller)
	if err != nil || stopped.State != sandboxdomain.StateStopped {
		t.Fatalf("stop: %+v %v", stopped, err)
	}
	started, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStart, caller)
	if err != nil || started.State != sandboxdomain.StateReady || started.Generation <= run.Generation {
		t.Fatalf("restart fenced generation: %+v %v", started, err)
	}
	destroyed, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxDestroy, caller)
	if err != nil || destroyed.State != sandboxdomain.StateDestroyed {
		t.Fatalf("destroy: %+v %v", destroyed, err)
	}
	if err := runtime.RemoveSandboxPluginInstallation(ctx, installation.ID()); err == nil {
		t.Fatal("destroyed retained Sandbox reference allowed plugin removal")
	}
	// Database-level guards close the check/disable/remove race between
	// concurrent runtimes, including direct store writes.
	if err := runtime.pluginInstallations.SetStatus(ctx, installation.ID(), installationdomain.StatusDisabled); err == nil {
		t.Fatal("direct installation store disabled referenced Sandbox driver")
	}
	if err := runtime.pluginInstallations.Delete(ctx, installation.ID()); err == nil {
		t.Fatal("direct installation store removed referenced Sandbox driver")
	}
	events, err := runtime.Sandboxes.store.ListLifecycleEvents(ctx, item.ID(), 10)
	if err != nil || len(events) < 3 {
		t.Fatalf("missing lifecycle audit %+v %v", events, err)
	}
}

func TestExternalSandboxUnknownProvisionRequiresGovernedReconcile(t *testing.T) {
	ctx := context.Background()
	pkg := exampleSandboxPluginPackage(t)
	folder := t.TempDir()
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(filepath.Join(folder, "plugins")) })
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(folder, "mender.db"), ProviderPluginRoot: filepath.Join(folder, "plugins")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	installed, err := runtime.InstallSandboxPlugin(ctx, pkg)
	if err != nil {
		t.Fatal(err)
	}
	caller := ownerContext(t, runtime)
	item, err := runtime.Sandboxes.CreateExternalSandbox(ctx,
		fixtureSandboxRequest(installed.ID(), `{"uncertainProvision":true}`), caller)
	if item.ID() == "" || err == nil {
		t.Fatalf("uncertain outcome not reported: item=%+v err=%v", item, err)
	}
	pending, err := runtime.Sandboxes.GetRun(ctx, item.ID())
	if err != nil || pending.State != sandboxdomain.StateUnknown {
		t.Fatalf("uncertain run not retained: %+v %v", pending, err)
	}
	place, err := runtime.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Sandboxes.router.Acquire(ctx, caller, place); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("unknown remote run executed: %v", err)
	}
	observed, err := runtime.Sandboxes.ReconcileExternalSandbox(ctx, item.ID(), caller)
	if err != nil || observed.State != sandboxdomain.StateReady {
		t.Fatalf("reconcile: %+v %v", observed, err)
	}
	events, err := runtime.Sandboxes.store.ListLifecycleEvents(ctx, item.ID(), 5)
	if err != nil || len(events) < 2 || events[0].Action != governancedomain.ActionSandboxReconcile {
		t.Fatalf("reconciliation lacked owner-attributed audit %+v %v", events, err)
	}
}

// Managed package permissions are intentionally read-only. The test owns
// these temporary paths exclusively and restores writable directories before
// testing.TempDir deletes its fixture, never touching user-installed plugins.
func makeManagedPluginTempTreeWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			// #nosec G122,G302 -- only a test-owned TempDir installation is walked; restore directory write access for TempDir teardown.
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
}

// A policy rejection must occur before the third-party executable starts and
// before any Sandbox desired-state or billable resource is persisted.
func TestExternalSandboxPolicyDenialNeverStartsPlugin(t *testing.T) {
	ctx := context.Background()
	pkg := exampleSandboxPluginPackage(t)
	folder := t.TempDir()
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(filepath.Join(folder, "plugins")) })
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(folder, "mender.db"), ProviderPluginRoot: filepath.Join(folder, "plugins")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	item, err := runtime.InstallSandboxPlugin(ctx, pkg)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := runtime.Policy.CreateRule.Execute(ctx, policyapp.RuleInput{
		DisplayName: "deny all Sandbox provisioning", Enabled: true, Decision: policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxProvision}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Policy.DeleteRule.Execute(ctx, rule.ID()) }()
	_, err = runtime.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(item.ID(), `{}`), ownerContext(t, runtime))
	if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("Policy denial bypassed: %v", err)
	}
	if runtime.externalProcesses.Running(item.ID()) {
		t.Fatal("policy denial launched external process")
	}
	resources, err := runtime.Sandboxes.List(ctx)
	if err != nil || len(resources) != 0 {
		t.Fatalf("policy-denied Sandbox persisted %+v %v", resources, err)
	}
}

// When an external binary/runtime restarts, its verified package metadata and
// resource binding are restored process-free. The fake backend intentionally
// loses in-memory state: Mender must not create a substitute host Sandbox or
// silently rerun provisioning.
func TestExternalSandboxRuntimeRestartCannotReplayOrFallBack(t *testing.T) {
	ctx := context.Background()
	pkg := exampleSandboxPluginPackage(t)
	folder := t.TempDir()
	managed := filepath.Join(folder, "plugins")
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(managed) })
	database := filepath.Join(folder, "mender.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database, ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := runtime.InstallSandboxPlugin(ctx, pkg)
	if err != nil {
		t.Fatal(err)
	}
	item, err := runtime.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(installed.ID(), `{}`), ownerContext(t, runtime))
	if err != nil {
		t.Fatal(err)
	}
	original, err := runtime.Sandboxes.GetRun(ctx, item.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database, ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	if restarted.externalProcesses.Running(installed.ID()) {
		t.Fatal("restart catalogue launched external plugin")
	}
	loaded, err := restarted.Sandboxes.Get(ctx, item.ID())
	if err != nil || loaded.Spec() != item.Spec() {
		t.Fatalf("restart altered pinned Sandbox: %+v %v", loaded.Spec(), err)
	}
	run, err := restarted.Sandboxes.GetRun(ctx, item.ID())
	if err != nil || run != original {
		t.Fatalf("restart altered immutable run: %+v %+v %v", run, original, err)
	}
	place, err := restarted.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.externalProcesses.Running(installed.ID()) {
		t.Fatal("read-only Router.Resolve launched plugin")
	}
	_, err = restarted.Sandboxes.router.Acquire(ctx, ownerContext(t, restarted), place)
	if err == nil {
		t.Fatal("lost fake remote state silently resumed or fell back to host")
	}
	leases, err := restarted.Sandboxes.store.ActiveLeaseCount(ctx, item.ID())
	if err != nil || leases != 0 {
		t.Fatalf("failed remote acquire stranded lease=%d %v", leases, err)
	}
	after, err := restarted.Sandboxes.GetRun(ctx, item.ID())
	if err != nil || after != original {
		t.Fatalf("failed acquire re-provisioned resource: %+v %v", after, err)
	}
}

// Tampered package bytes fail before the external code is executed.
func TestExternalSandboxPackageTamperFailsClosed(t *testing.T) {
	ctx := context.Background()
	pkg := exampleSandboxPluginPackage(t)
	folder := t.TempDir()
	managed := filepath.Join(folder, "plugins")
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(managed) })
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(folder, "mender.db"), ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	item, err := runtime.InstallSandboxPlugin(ctx, pkg)
	if err != nil {
		t.Fatal(err)
	}
	binName := "mender-sandbox-example"
	if goruntime.GOOS == "windows" {
		binName += ".exe"
	}
	executable := filepath.Join(managed, item.ID().String(), "bin", binName)
	// Only modify this test's installation, never an installed user package.
	if err := os.Chmod(executable, 0o600); err != nil {
		t.Fatal(err)
	} // #nosec G302 -- temporarily writable test-owned fixture.
	file, err := os.OpenFile(executable, os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304,G302 -- fixed temp package path.
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("modified")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("mutate fixture: %v %v", writeErr, closeErr)
	}
	if err := os.Chmod(executable, 0o500); err != nil {
		t.Fatal(err)
	} // #nosec G302 -- managed fixture executable restored.
	if _, err := runtime.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(item.ID(), `{}`), ownerContext(t, runtime)); err == nil {
		t.Fatal("tampered external driver was executed")
	}
	if runtime.externalProcesses.Running(item.ID()) {
		t.Fatal("tampered external plugin process launched")
	}
	items, err := runtime.Sandboxes.List(ctx)
	if err != nil || len(items) != 0 {
		t.Fatalf("tampered package provisioned resources: %+v %v", items, err)
	}
}

// The real Workspace Provider (not a direct backend session) invokes the
// external Sandbox through Catalog -> Invoke -> Provider -> Sandbox Router,
// with the outer Tool audit and Sandbox durable lease enforced independently.
func TestExternalSandboxWorkspaceProviderCanonicalToolInvocation(t *testing.T) {
	ctx := context.Background()
	pkg := exampleSandboxPluginPackage(t)
	folder := t.TempDir()
	managed := filepath.Join(folder, "managed")
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(managed) })
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(folder, "mender.db"), ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	installation, err := r.InstallSandboxPlugin(ctx, pkg)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := r.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(installation.ID(), `{}`), ownerContext(t, r))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(workspaceplugin.Config{SandboxID: sandbox.ID().String()})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
		Namespace: "external-sandbox-workspace", DisplayName: "External Sandbox Workspace",
	}, workspaceplugin.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfiguredProviderTest(t, r, ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	tools, err := r.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	var readToolFound bool
	for _, tool := range tools {
		if tool.SourceKey() != "workspace.read" {
			continue
		}
		readToolFound = true
		response, err := r.CallConfiguredTool(ctx, tool.ID(), []byte(`{"path":"README.txt"}`), ownerContext(t, r))
		if err != nil || response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("canonical Workspace Tool did not execute inside plugin Sandbox: outcome=%+v err=%v", response.Outcome, err)
		}
		var result struct {
			Path, Content      string
			StartLine, EndLine int
			Truncated          bool
		}
		if err := json.Unmarshal(response.Outcome.Content().Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Path != "README.txt" || !strings.Contains(result.Content, "Example Sandbox") || result.StartLine != 1 ||
			result.EndLine != 1 || result.Truncated {
			t.Fatalf("unexpected canonical read result %+v", result)
		}
		audit, err := r.audit.Get(ctx, response.InvocationID)
		if err != nil || audit.ProviderID() == nil || *audit.ProviderID() != provider.ID() ||
			audit.OutcomeKind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("canonical Invocation audit not persisted: %+v %v", audit, err)
		}
		leases, err := r.Sandboxes.store.ActiveLeaseCount(ctx, sandbox.ID())
		if err != nil || leases != 0 {
			t.Fatalf("Tool Invocation leaked Sandbox lease: %d %v", leases, err)
		}
		// Denied acquisition must never reach the native session.
		rule, err := r.Policy.CreateRule.Execute(ctx, policyapp.RuleInput{
			DisplayName: "deny external Sandbox acquisition",
			Enabled:     true, Decision: policydomain.DecisionDeny,
			Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxAcquire}},
		})
		if err != nil {
			t.Fatal(err)
		}
		second, secondErr := r.CallConfiguredTool(ctx, tool.ID(), []byte(`{"path":"README.txt"}`), ownerContext(t, r))
		if secondErr == nil && second.Outcome.Kind() == invocationdomain.OutcomeKindComplete {
			t.Fatal("policy-denied external Sandbox acquire executed Workspace Tool")
		}
		if err := r.Policy.DeleteRule.Execute(ctx, rule.ID()); err != nil {
			t.Fatal(err)
		}
		break
	}
	if !readToolFound {
		t.Fatal("canonical workspace.read Tool not discovered")
	}
}

func TestExternalSandboxCannotCreateWithDisabledPackage(t *testing.T) {
	ctx := context.Background()
	source := exampleSandboxPluginPackage(t)
	folder := t.TempDir()
	managed := filepath.Join(folder, "plugins")
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(managed) })
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(folder, "mender.db"), ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	installed, err := r.InstallSandboxPlugin(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetSandboxPluginInstallationStatus(ctx, installed.ID(), installationdomain.StatusDisabled); err != nil {
		t.Fatalf("unused driver could not be disabled: %v", err)
	}
	// Model a racing creator that already read the descriptor before a
	// different Runtime disabled the plugin. Atomic INSERT must still fail.
	caller := ownerContext(t, r)
	subject := caller.Principal()
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID:   "sbx_0123456789abcdef0123456789abcdef",
		Name: "Race-fenced driver", Backend: sandboxdomain.BackendExternal,
		Binding:   sandboxdomain.BackendBinding{PluginID: installed.PluginID(), ExtensionID: "main", InstallationID: installed.ID()},
		Isolation: sandboxdomain.IsolationRemote,
		OwnerKind: string(subject.Kind()), OwnerRef: subject.Reference(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Sandboxes.store.CreateDefinedExternal(ctx, item, sandboxsqlite.ExternalConfig{
		ConfigVersion: 1, JSON: json.RawMessage(`{}`),
	}); err == nil {
		t.Fatal("disabled driver created Sandbox despite atomic SQLite pin check")
	}
	if r.externalProcesses.Running(installed.ID()) {
		t.Fatal("disabled driver process started")
	}
	inventory, err := r.Sandboxes.List(ctx)
	if err != nil || len(inventory) != 0 {
		t.Fatalf("disabled driver inserted resources: %+v %v", inventory, err)
	}
}

// Both runtimes share one persisted SQLite lease. Contention is rejected
// before the second Runtime opens the third-party native backend process.
func TestExternalSandboxLeaseContentionAcrossRuntimes(t *testing.T) {
	ctx := context.Background()
	source := exampleSandboxPluginPackage(t)
	folder := t.TempDir()
	managed := filepath.Join(folder, "plugins")
	t.Cleanup(func() { makeManagedPluginTempTreeWritable(managed) })
	database := filepath.Join(folder, "mender.db")
	first, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database, ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	installed, err := first.InstallSandboxPlugin(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	item, err := first.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(installed.ID(), `{}`), ownerContext(t, first))
	if err != nil {
		t.Fatal(err)
	}
	place, err := first.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := first.Sandboxes.router.Acquire(ctx, ownerContext(t, first), place)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Close() }()

	second, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database, ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	secondPlace, err := second.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if second.externalProcesses.Running(installed.ID()) {
		t.Fatal("second Runtime started plugin during metadata resolution")
	}
	if _, err := second.Sandboxes.router.Acquire(ctx, ownerContext(t, second), secondPlace); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("cross-Runtime durable execution lease bypassed: %v", err)
	}
	if second.externalProcesses.Running(installed.ID()) {
		t.Fatal("conflicted acquire launched second plugin instance")
	}
	count, err := second.Sandboxes.store.ActiveLeaseCount(ctx, item.ID())
	if err != nil || count != 1 {
		t.Fatalf("cross-Runtime lease count %d %v", count, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	count, err = second.Sandboxes.store.ActiveLeaseCount(ctx, item.ID())
	if err != nil || count != 0 {
		t.Fatalf("durable lease release failed across Runtimes: %d %v", count, err)
	}
}
