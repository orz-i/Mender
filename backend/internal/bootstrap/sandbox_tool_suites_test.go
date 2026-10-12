package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	suitedomain "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func newSuiteRuntime(t *testing.T) *Runtime {
	t.Helper()
	r, err := NewRuntime(context.Background(), RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.sqlite"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func createSuiteSandbox(t *testing.T, r *Runtime, name string) sandboxdomain.Sandbox {
	t.Helper()
	sandbox, err := r.Sandboxes.CreateLocalSandbox(context.Background(), CreateLocalSandboxRequest{
		Name: name, Root: t.TempDir(), Isolation: sandboxdomain.IsolationHost,
	}, agentOwnerContext(t, r))
	if err != nil {
		t.Fatal(err)
	}
	return sandbox
}

func TestManagedSandboxSuiteDistinctStableAndNoImplicitExposure(t *testing.T) {
	ctx := context.Background()
	r := newSuiteRuntime(t)
	owner := agentOwnerContext(t, r)
	definitions, err := r.ListToolSuiteDefinitions(ctx)
	if err != nil || len(definitions) != 3 {
		t.Fatalf("definitions=%+v err=%v", definitions, err)
	}
	if definitions[2].SuiteID != suitedomain.SandboxBrowser || definitions[2].Status != "unavailable" ||
		len(definitions[2].ToolSourceKeys) != 0 {
		t.Fatalf("browser claims tools: %+v", definitions[2])
	}
	providersBefore, err := r.Catalog.ListProviders.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Build Workspace", "Documentation"} {
		sandbox := createSuiteSandbox(t, r, name)
		before, err := r.SandboxWorkspaceTools(ctx, sandbox.ID(), owner)
		if err != nil || before.Status != "missing" || !before.Available || len(before.Tools) != 0 {
			t.Fatalf("inert empty suite=%+v err=%v", before, err)
		}
		providersNow, err := r.Catalog.ListProviders.Execute(ctx)
		if err != nil || len(providersNow) != len(providersBefore) {
			t.Fatal("suite GET created a Provider")
		}
		first, err := r.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), owner)
		if err != nil || first.Status != "ready" || len(first.Tools) != 4 || first.ProviderID == "" {
			t.Fatalf("ensure=%+v err=%v", first, err)
		}
		second, err := r.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), owner)
		if err != nil || second.ProviderID != first.ProviderID {
			t.Fatalf("repeated ensure changed Provider identity: %+v %v", second, err)
		}
		for i, tool := range first.Tools {
			if tool.ToolID == "" || second.Tools[i].ToolID != tool.ToolID {
				t.Fatal("Tool identity changed on ensure")
			}
		}
		pid, err := catalogdomain.ParseProviderID(first.ProviderID)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.DeleteConfiguredProvider(ctx, pid); !errors.Is(err, ErrProviderInUse) {
			t.Fatalf("generic Provider delete escaped ownership: %v", err)
		}
		profiles, err := r.Profiles.List.Execute(ctx)
		if err != nil || len(profiles) != 0 {
			t.Fatal("suite was exposed to a Profile")
		}
		if err := r.RemoveSandboxWorkspaceTools(ctx, sandbox.ID(), owner); err != nil {
			t.Fatalf("unexposed suite remove: %v", err)
		}
		missing, err := r.SandboxWorkspaceTools(ctx, sandbox.ID(), owner)
		if err != nil || missing.ProviderID != "" || missing.Status != "missing" {
			t.Fatalf("suite not retired: %+v %v", missing, err)
		}
	}
}

func TestSandboxSuiteAdoptsLegacyIdentityAndRejectsDuplicates(t *testing.T) {
	ctx := context.Background()
	r := newSuiteRuntime(t)
	owner := agentOwnerContext(t, r)
	sandbox := createSuiteSandbox(t, r, "Review Workspace")
	raw, _ := json.Marshal(workspaceplugin.Config{SandboxID: sandbox.ID().String()})
	legacy, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
		Namespace: "workspace-original", DisplayName: "Original Workspace",
	}, workspaceplugin.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.SyncConfiguredProvider(ctx, legacy.ID(), owner); err != nil {
		t.Fatal(err)
	}
	before, err := r.SandboxWorkspaceTools(ctx, sandbox.ID(), owner)
	if err != nil || before.Status != "legacy" {
		t.Fatalf("legacy missing: %+v %v", before, err)
	}
	adopted, err := r.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), owner)
	if err != nil || adopted.ProviderID != legacy.ID().String() || adopted.Status != "ready" {
		t.Fatalf("legacy provider replaced: %+v %v", adopted, err)
	}
	for i, tool := range before.Tools {
		if adopted.Tools[i].ToolID != tool.ToolID {
			t.Fatal("adopted ToolID changed")
		}
	}
	if err := r.RemoveSandboxWorkspaceTools(ctx, sandbox.ID(), owner); err != nil {
		t.Fatal(err)
	}

	secondSandbox := createSuiteSandbox(t, r, "Duplicate Workspace")
	config, _ := json.Marshal(workspaceplugin.Config{SandboxID: secondSandbox.ID().String()})
	for _, ns := range []string{"workspace-first", "workspace-second"} {
		if _, err = registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
			Namespace: ns, DisplayName: ns,
		}, workspaceplugin.ID, config); err != nil {
			t.Fatal(err)
		}
	}
	conflict, err := r.SandboxWorkspaceTools(ctx, secondSandbox.ID(), owner)
	if err != nil || conflict.Status != "conflict" {
		t.Fatalf("not a conflict: %+v %v", conflict, err)
	}
	if _, err = r.EnsureSandboxWorkspaceTools(ctx, secondSandbox.ID(), owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("multiple Providers silently merged: %v", err)
	}
}

func TestSandboxSuiteConcurrentClaimsUniqueAndProfileDenied(t *testing.T) {
	ctx := context.Background()
	r := newSuiteRuntime(t)
	sandbox := createSuiteSandbox(t, r, "Concurrent")
	raw, _ := json.Marshal(workspaceplugin.Config{SandboxID: sandbox.ID().String()})
	ids := []string{}
	for _, ns := range []string{"race-one", "race-two"} {
		provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
			Namespace: ns, DisplayName: ns,
		}, workspaceplugin.ID, raw)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, provider.ID().String())
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	wg.Add(len(ids))
	for _, id := range ids {
		go func() {
			defer wg.Done()
			err := r.Sandboxes.store.ClaimWorkspaceSuite(ctx, sandbox.ID(), id)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
			} else if !errors.Is(err, sandboxdomain.ErrConflict) {
				t.Errorf("unexpected concurrent claim: %v", err)
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("expected one authoritative claim, got %d", success)
	}
	serviceCtx := taskServicePrincipal(t, "prof_00000000000000000000000000001234")
	if _, err := r.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), serviceCtx); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("Profile could create suite: %v", err)
	}
	if _, err := r.SandboxWorkspaceTools(ctx, sandbox.ID(), serviceCtx); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("Profile could inspect management-only binding: %v", err)
	}
}
