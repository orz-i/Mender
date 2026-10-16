package bootstrap

import (
	"context"
	"errors"
	"testing"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestWorkspaceToolMultiTargetReusesProviderAndRejectsUnauthorizedSelection(t *testing.T) {
	ctx := context.Background()
	r := newSuiteRuntime(t)
	owner := agentOwnerContext(t, r)
	defaultSandbox := createSuiteSandbox(t, r, "Default project")
	alternate := createSuiteSandbox(t, r, "Alternate project")
	unclaimed := createSuiteSandbox(t, r, "Unclaimed service")
	suite, err := r.EnsureSandboxWorkspaceTools(ctx, defaultSandbox.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(suite.Tools) != 4 {
		t.Fatalf("expected four stable tools: %+v", suite)
	}
	ids := make([]string, len(suite.Tools))
	for i, item := range suite.Tools {
		ids[i] = item.ToolID
	}
	if _, err := r.ListWorkspaceToolTargets(ctx, suite.ProviderID, taskServicePrincipal(t, "prof_00000000000000000000000000000001")); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("profile read owner target inventory: %v", err)
	}
	if err := r.BindWorkspaceToolTarget(ctx, suite.ProviderID, "review", alternate.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	targets, err := r.ListWorkspaceToolTargets(ctx, suite.ProviderID, owner)
	if err != nil || len(targets) != 1 || targets[0].SandboxID != alternate.ID().String() {
		t.Fatalf("targets=%+v err=%v", targets, err)
	}
	selected, err := r.Sandboxes.ResolveWorkspaceTarget(ctx, suite.ProviderID, "review")
	if err != nil || selected != alternate.ID() {
		t.Fatalf("target=%s err=%v", selected, err)
	}
	if _, err := r.Sandboxes.ResolveWorkspaceTarget(ctx, suite.ProviderID, "elsewhere"); !errors.Is(err, sandboxdomain.ErrNotFound) {
		t.Fatalf("unexpected missing alias: %v", err)
	}
	if err := r.BindWorkspaceToolTarget(ctx, suite.ProviderID, "other", alternate.ID().String(), owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("duplicate target accepted: %v", err)
	}
	if err := r.BindWorkspaceToolTarget(ctx, suite.ProviderID, "default", unclaimed.ID().String(), owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("invalid alias accepted: %v", err)
	}
	if err := r.BindWorkspaceToolTarget(ctx, suite.ProviderID, "self", defaultSandbox.ID().String(), owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("duplicate default accepted: %v", err)
	}
	if err := r.BindWorkspaceToolTarget(ctx, "provider_00000000000000000000000000000000", "other", unclaimed.ID().String(), owner); err == nil {
		t.Fatal("unclaimed source accepted")
	}
	suite2, err := r.EnsureSandboxWorkspaceTools(ctx, defaultSandbox.ID(), owner)
	if err != nil || suite2.ProviderID != suite.ProviderID {
		t.Fatalf("provider identity mutated: %+v %v", suite2, err)
	}
	for i, item := range suite2.Tools {
		if item.ToolID != ids[i] {
			t.Fatal("Target registration changed Tool ID")
		}
	}
	if err := r.RemoveSandboxWorkspaceTools(ctx, defaultSandbox.ID(), owner); err == nil {
		t.Fatal("retired a Tool Suite with additional workspace targets")
	}
	if err := r.UnbindWorkspaceToolTarget(ctx, suite.ProviderID, "review", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Sandboxes.ResolveWorkspaceTarget(ctx, suite.ProviderID, "review"); !errors.Is(err, sandboxdomain.ErrNotFound) {
		t.Fatalf("removed target still resolvable: %v", err)
	}
	if err := r.RemoveSandboxWorkspaceTools(ctx, defaultSandbox.ID(), owner); err != nil {
		t.Fatal(err)
	}
}
