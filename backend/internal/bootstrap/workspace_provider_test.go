package bootstrap

import (
	"context"
	"encoding/json"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	"os"
	"path/filepath"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func TestWorkspaceProviderRuntimeComposesProfilePolicyAndAudit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "notes.txt"),
		[]byte("hello workspace\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	rawConfig, err := json.Marshal(workspaceplugin.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := runtime.RegisterConfiguredProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "local-workspace",
			DisplayName: "Local Workspace",
		},
		workspaceplugin.ID,
		rawConfig,
	)
	if err != nil {
		t.Fatalf("RegisterConfiguredProvider() error = %v", err)
	}

	firstSync, err := runtime.SyncConfiguredProvider(ctx, provider.ID())
	if err != nil {
		t.Fatalf("SyncConfiguredProvider() error = %v", err)
	}
	if firstSync.Total != 4 || firstSync.Created != 4 {
		t.Fatalf("first sync = %+v, want 4 created Tools", firstSync)
	}

	firstTools, err := runtime.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	firstIDs := make(map[string]catalogdomain.ToolID, len(firstTools))
	for _, tool := range firstTools {
		firstIDs[tool.SourceKey()] = tool.ID()
	}
	for _, sourceKey := range []string{
		"workspace.read",
		"workspace.search",
		"workspace.edit",
		"workspace.shell",
	} {
		if _, ok := firstIDs[sourceKey]; !ok {
			t.Fatalf("workspace Tool %q missing from synchronized Catalog: %v", sourceKey, firstIDs)
		}
	}

	secondSync, err := runtime.SyncConfiguredProvider(ctx, provider.ID())
	if err != nil {
		t.Fatalf("second SyncConfiguredProvider() error = %v", err)
	}
	if secondSync.Total != 4 || secondSync.Updated != 4 || secondSync.Created != 0 {
		t.Fatalf("second sync = %+v, want four stable updates", secondSync)
	}
	secondTools, err := runtime.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range secondTools {
		if firstIDs[tool.SourceKey()] != tool.ID() {
			t.Fatalf(
				"ToolID changed for %s: %s -> %s",
				tool.SourceKey(),
				firstIDs[tool.SourceKey()],
				tool.ID(),
			)
		}
	}

	profile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "workspace-editor",
			DisplayName: "Workspace Editor",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, sourceKey := range []string{"workspace.read", "workspace.edit"} {
		profile, err = runtime.Profiles.AddTool.Execute(
			ctx,
			profileapplication.AddToolRequest{
				ProfileSlug: profile.Slug(),
				ToolID:      firstIDs[sourceKey],
			},
		)
		if err != nil {
			t.Fatalf("AddTool(%s) error = %v", sourceKey, err)
		}
	}

	scope, err := governancedomain.ProfileScope(profile.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	invocationContext, err := governancedomain.NewContext(
		governancedomain.OriginInternal,
		testPrincipal(t, "workspace-runtime-test"),
		scope,
	)
	if err != nil {
		t.Fatal(err)
	}

	readResponse, err := runtime.CallConfiguredTool(
		ctx,
		firstIDs["workspace.read"],
		[]byte(`{"path":"notes.txt"}`),
		invocationContext,
	)
	if err != nil {
		t.Fatalf("workspace.read error = %v", err)
	}
	if readResponse.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("workspace.read outcome = %#v", readResponse.Outcome)
	}
	readAudit, err := runtime.audit.Get(ctx, readResponse.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if readAudit.ProviderID() == nil ||
		*readAudit.ProviderID() != provider.ID() ||
		readAudit.OutcomeKind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("workspace.read audit = %#v", readAudit)
	}

	shellResponse, err := runtime.CallConfiguredTool(
		ctx,
		firstIDs["workspace.shell"],
		[]byte(`{"command":"echo should-not-run"}`),
		invocationContext,
	)
	if err != nil {
		t.Fatalf("out-of-profile workspace.shell error = %v", err)
	}
	if shellResponse.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		shellResponse.Outcome.RejectionCode() != invocationdomain.RejectionToolOutsideScope {
		t.Fatalf("out-of-profile workspace.shell outcome = %#v", shellResponse.Outcome)
	}

	resourceMatcher, err := policydomain.NewResourceMatcher(
		"workspace_path",
		policydomain.ResourceExact,
		"blocked.txt",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{
		DisplayName: "Block one workspace write",
		Enabled:     true,
		Decision:    policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{
			Actions:       []governancedomain.Action{governancedomain.ActionToolInvoke},
			ProviderIDs:   []string{provider.ID().String()},
			FactKnowledge: policydomain.FactKnown,
			EffectsAny:    []string{"filesystem.write"},
			ResourcesAny:  []policydomain.ResourceMatcher{resourceMatcher},
		},
	}); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	editResponse, err := runtime.CallConfiguredTool(
		ctx,
		firstIDs["workspace.edit"],
		[]byte(`{"path":"blocked.txt","operation":"create","content":"denied"}`),
		invocationContext,
	)
	if err != nil {
		t.Fatalf("workspace.edit denied call error = %v", err)
	}
	if editResponse.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		editResponse.Outcome.RejectionCode() != invocationdomain.RejectionPolicyDenied {
		t.Fatalf("workspace.edit denied outcome = %#v", editResponse.Outcome)
	}
	if _, err := os.Stat(filepath.Join(root, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("policy-denied edit reached filesystem; Stat() error = %v", err)
	}
}
