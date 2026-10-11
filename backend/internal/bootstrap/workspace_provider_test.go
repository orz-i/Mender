package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

func TestWorkspaceSandboxIsolationIsPinnedAcrossGlobalUpdates(t *testing.T) {
	ctx := context.Background()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, ".mender-workspace-isolation-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	secretRoot, err := os.MkdirTemp(home, ".mender-workspace-secret-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(secretRoot) }()
	secret := filepath.Join(secretRoot, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP-SECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	if !runtime.RuntimeGovernance.processIsolation.Info().RestrictedHostAvailable {
		t.Skip("restricted_host backend unavailable")
	}

	if _, err := runtime.RuntimeGovernance.UpdateIsolation(
		ctx,
		runtimegovernancedomain.IsolationSpec{
			WorkspaceShell:   runtimegovernancedomain.IsolationRestrictedHost,
			ExternalProvider: runtimegovernancedomain.IsolationHost,
			TunnelClient:     runtimegovernancedomain.IsolationHost,
		},
	); err != nil {
		t.Fatal(err)
	}

	rawConfig := testWorkspaceSandboxConfig(t, runtime, root, testPrincipal(t, "workspace-isolation"), sandboxdomain.IsolationRestrictedHost)
	provider, err := registerConfiguredProviderTest(
		t,
		runtime,
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "isolation-workspace",
			DisplayName: "Isolation Workspace",
		},
		workspaceplugin.ID,
		rawConfig,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfiguredProviderTest(t, runtime, ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	tools, err := runtime.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	var shellID catalogdomain.ToolID
	for _, tool := range tools {
		if tool.SourceKey() == "workspace.shell" {
			shellID = tool.ID()
			break
		}
	}
	if shellID == "" {
		t.Fatal("workspace.shell Tool not found")
	}
	governanceContext := testCatalogGovernanceContext(
		t,
		governancedomain.OriginInternal,
		"workspace-isolation",
	)

	command := "if /bin/cat " + shellDoubleQuote(secret) +
		" >/dev/null 2>&1; then echo leaked; exit 77; else echo blocked; fi; echo ok > inside.txt"
	arguments, err := json.Marshal(map[string]any{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := runtime.CallConfiguredTool(ctx, shellID, arguments, governanceContext)
	if err != nil {
		t.Fatal(err)
	}
	if restricted.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("restricted shell outcome = %#v", restricted.Outcome)
	}
	var restrictedOutput struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
	}
	if err := json.Unmarshal(restricted.Outcome.Content().Bytes(), &restrictedOutput); err != nil {
		t.Fatal(err)
	}
	if restrictedOutput.ExitCode != 0 || !strings.Contains(restrictedOutput.Stdout, "blocked") || strings.Contains(restrictedOutput.Stdout, "leaked") {
		t.Fatalf("restricted shell output = %+v", restrictedOutput)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := rootHandle.ReadFile("inside.txt")
	_ = rootHandle.Close()
	if readErr != nil || string(data) != "ok\n" {
		t.Fatalf("restricted workspace write = %q error=%v", data, readErr)
	}

	if _, err := runtime.RuntimeGovernance.UpdateIsolation(
		ctx,
		runtimegovernancedomain.IsolationSpec{
			WorkspaceShell:   runtimegovernancedomain.IsolationHost,
			ExternalProvider: runtimegovernancedomain.IsolationHost,
			TunnelClient:     runtimegovernancedomain.IsolationHost,
		},
	); err != nil {
		t.Fatal(err)
	}
	hostArgs, err := json.Marshal(map[string]any{
		"command": "/bin/cat " + shellDoubleQuote(secret),
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := runtime.CallConfiguredTool(ctx, shellID, hostArgs, governanceContext)
	if err != nil {
		t.Fatal(err)
	}
	var hostOutput struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
	}
	if err := json.Unmarshal(host.Outcome.Content().Bytes(), &hostOutput); err != nil {
		t.Fatal(err)
	}
	if hostOutput.ExitCode == 0 || strings.Contains(hostOutput.Stdout, "TOP-SECRET") {
		t.Fatalf("Sandbox isolation was downgraded by global setting: %+v", hostOutput)
	}
}

func shellDoubleQuote(value string) string {
	return "\"" + strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"$", "\\$",
		"`", "\\`",
	).Replace(value) + "\""
}

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

	rawConfig := testWorkspaceSandboxConfig(t, runtime, root, testPrincipal(t, "workspace-runtime-test"), sandboxdomain.IsolationHost)
	provider, err := registerConfiguredProviderTest(t, runtime,
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

	firstSync, err := syncConfiguredProviderTest(t, runtime, ctx, provider.ID())
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

	secondSync, err := syncConfiguredProviderTest(t, runtime, ctx, provider.ID())
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

func testWorkspaceSandboxConfig(t *testing.T, runtime *Runtime, root string, principal governancedomain.Principal, isolation sandboxdomain.IsolationClass) json.RawMessage {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	id, err := sandboxdomain.ParseID("sbx_" + hex.EncodeToString(random[:]))
	if err != nil {
		t.Fatal(err)
	}
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Test workspace", Backend: sandboxdomain.BackendLocalWorkspace, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace), Isolation: isolation, LocalRoot: canonical, OwnerKind: string(principal.Kind()), OwnerRef: principal.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Sandboxes.store.Create(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "local_workspace:"+id.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Sandboxes.store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(workspaceplugin.Config{SandboxID: id.String()})
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func TestWorkspaceProviderRegistrationRejectsDanglingSandbox(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	raw, err := json.Marshal(workspaceplugin.Config{SandboxID: "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = registerConfiguredProviderTest(t, runtime, ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "missing-sandbox", DisplayName: "Missing Sandbox Workspace",
	}, workspaceplugin.ID, raw)
	if !errors.Is(err, sandboxdomain.ErrNotFound) {
		t.Fatalf("invalid Sandbox was configured: %v", err)
	}
	configured, err := runtime.ListProviderConfigs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(configured) != 0 {
		t.Fatalf("failed Provider activation persisted config: %+v", configured)
	}
}
