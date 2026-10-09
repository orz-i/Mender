package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func localSandboxCaller(t *testing.T, r *Runtime) governancedomain.Context {
	t.Helper()
	caller, err := governancedomain.NewContext(governancedomain.OriginInternal, r.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return caller
}
func createdLocalSandbox(t *testing.T, r *Runtime, root string) (sandboxdomain.Sandbox, governancedomain.Context) {
	t.Helper()
	caller := localSandboxCaller(t, r)
	item, err := r.Sandboxes.CreateLocalSandbox(context.Background(), CreateLocalSandboxRequest{Name: "Lifecycle local", Root: root, Isolation: sandboxdomain.IsolationHost}, caller)
	if err != nil {
		t.Fatal(err)
	}
	return item, caller
}
func TestSandboxLocalLifecycleDurableLeasesAcrossRuntimes(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "lifecycle.db")
	r1, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r1.Close() }()
	item, caller := createdLocalSandbox(t, r1, t.TempDir())
	r2, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r2.Close() }()
	placement, err := r1.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := r1.Sandboxes.router.Acquire(ctx, caller, placement)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r2.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStop, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stop while execution active: %v", err)
	}
	if _, err := r2.Sandboxes.router.Acquire(ctx, caller, placement); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("second Runtime acquired same Sandbox: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	stopped, err := r2.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStop, caller)
	if err != nil || stopped.State != sandboxdomain.StateStopped || stopped.Generation != 1 {
		t.Fatalf("stop %+v %v", stopped, err)
	}
	if _, err := r1.Sandboxes.router.Acquire(ctx, caller, placement); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("stopped sandbox acquired via stale placement: %v", err)
	}
	started, err := r1.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStart, caller)
	if err != nil || started.State != sandboxdomain.StateReady || started.Generation != 2 {
		t.Fatalf("start %+v %v", started, err)
	}
	if _, err := r1.Sandboxes.router.Acquire(ctx, caller, placement); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("prior generation reused: %v", err)
	}
	newPlacement, err := r2.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	newLease, err := r2.Sandboxes.router.Acquire(ctx, caller, newPlacement)
	if err != nil {
		t.Fatal(err)
	}
	if err := newLease.Close(); err != nil {
		t.Fatal(err)
	}
	events, err := r2.Sandboxes.LifecycleEvents(ctx, item.ID(), 10)
	if err != nil || len(events) != 2 {
		t.Fatalf("events %+v %v", events, err)
	}
	if events[0].Action != governancedomain.ActionSandboxStart || events[0].Generation != 2 || events[1].Action != governancedomain.ActionSandboxStop {
		t.Fatalf("event order %+v", events)
	}
}

func TestSandboxDestroyConflictsWithWorkspaceProviderAndPreservesHostFiles(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "reference.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	root := t.TempDir()
	file := filepath.Join(root, "important.txt")
	if err := os.WriteFile(file, []byte("keep this"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, caller := createdLocalSandbox(t, r, root)
	raw, err := json.Marshal(workspaceplugin.Config{SandboxID: item.ID().String()})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := r.RegisterConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{Namespace: "lifecycle-reference", DisplayName: "Workspace"}, workspaceplugin.ID, raw, caller)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governancedomain.ActionSandboxDestroy, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("referenced Sandbox destroyed: %v", err)
	}
	if err := r.providerConfigs.Delete(ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	destroyed, err := r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governancedomain.ActionSandboxDestroy, caller)
	if err != nil || destroyed.State != sandboxdomain.StateDestroyed {
		t.Fatalf("destroy %+v %v", destroyed, err)
	}
	// #nosec G304 -- test fixture file path is constructed under t.TempDir().
	contents, err := os.ReadFile(file)
	if err != nil || string(contents) != "keep this" {
		t.Fatalf("destroy removed host files: %q %v", contents, err)
	}
	if _, err := r.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("destroyed run still routable: %v", err)
	}
	v2, err := providerplugin.NewConfiguration(provider.ID(), workspaceplugin.ID, 2, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.providerConfigs.Save(ctx, v2); err == nil {
		t.Fatal("raw DB writer bound Provider to destroyed Sandbox")
	}
	events, err := r.Sandboxes.LifecycleEvents(ctx, item.ID(), 10)
	if err != nil || len(events) != 1 || events[0].Action != governancedomain.ActionSandboxDestroy {
		t.Fatalf("destroy audit %+v %v", events, err)
	}
}

func TestSandboxLifecycleDeniedAndStartPreflightFailClosed(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "policy.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	root := t.TempDir()
	item, caller := createdLocalSandbox(t, r, root)
	rule, err := r.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{DisplayName: "deny stop", Enabled: true, Decision: policydomain.DecisionDeny, Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxStop}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStop, caller); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("policy bypass: %v", err)
	}
	events, err := r.Sandboxes.LifecycleEvents(ctx, item.ID(), 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("denied lifecycle wrote success audit %+v %v", events, err)
	}
	if err := r.Policy.DeleteRule.Execute(ctx, rule.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStop, caller); err != nil {
		t.Fatal(err)
	}
	// A disappeared root remains stopped; never downgrade to an alternate root.
	renamed := root + "-absent"
	if err := os.Rename(root, renamed); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Rename(renamed, root) }()
	if _, err := r.Sandboxes.ChangeLocalLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStart, caller); err == nil {
		t.Fatal("start accepted disappeared root")
	}
	after, err := r.Sandboxes.GetRun(ctx, item.ID())
	if err != nil || after.State != sandboxdomain.StateStopped {
		t.Fatalf("failed start mutated run %+v %v", after, err)
	}
}
