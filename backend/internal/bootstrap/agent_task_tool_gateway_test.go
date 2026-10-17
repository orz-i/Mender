package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type waitingForTaskToolExecutor struct {
	started  chan struct{}
	selected chan agentapp.ToolSupply
}

func (e waitingForTaskToolExecutor) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, cfg agentapp.RunConfig) (string, error) {
	if e.selected != nil {
		select {
		case e.selected <- cfg.ToolSupply:
		default:
		}
	}
	select {
	case e.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return "", ctx.Err()
}

type governedTaskToolFixture struct {
	runtime  *Runtime
	task     agentdomain.Task
	agent    agentdomain.Agent
	sandbox  sandboxdomain.Sandbox
	profile  profiledomain.Profile
	owner    gov.Context
	ids      map[string]catalogdomain.ToolID
	root     string
	token    string
	executor waitingForTaskToolExecutor
}

func setupGovernedTaskToolFixture(t *testing.T) *governedTaskToolFixture {
	t.Helper()
	ctx := context.Background()
	exec := waitingForTaskToolExecutor{started: make(chan struct{}, 2), selected: make(chan agentapp.ToolSupply, 2)}
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "governed-task-tools.db"),
		AgentExecutor: exec,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	owner := agentOwnerContext(t, r)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "review.txt"), []byte("governed temporary contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sandbox, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "Explicit restricted workspace", Root: root, Isolation: sandboxdomain.IsolationRestrictedHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := r.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if suite.Status != "ready" || len(suite.Tools) != 4 {
		t.Fatalf("bad Suite %+v", suite)
	}
	ids := make(map[string]catalogdomain.ToolID, len(suite.Tools))
	for _, tool := range suite.Tools {
		parsed, e := catalogdomain.ParseToolID(tool.ToolID)
		if e != nil {
			t.Fatal(e)
		}
		ids[tool.SourceKey] = parsed
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "agent-tools-test", DisplayName: "Agent Tool Scope", Mode: profiledomain.ExposureDirect,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"workspace.read", "workspace.edit", "workspace.shell"} {
		profile, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
			ProfileSlug: profile.Slug(), ToolID: ids[source], Direct: true,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Sandboxes.SetProfileGrants(ctx, sandbox.ID(), profile.ID().String(),
		[]sandboxdomain.Capability{sandboxdomain.CapabilityRead, sandboxdomain.CapabilityEdit}, owner); err != nil {
		t.Fatal(err)
	}
	agent, err := r.Agents.Create(ctx, agentapp.CreateRequest{
		Name: "Backend-independent Agent", Backend: agentdomain.BackendGrok, WorkspaceRoot: root, Isolation: "restricted_host",
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	caller := taskServicePrincipal(t, profile.ID().String())
	task, err := r.Agents.Submit(ctx, agent.ID(), "synthetic Task blocks until cancellation", caller)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-exec.started:
	case <-time.After(4 * time.Second):
		t.Fatal("Task never started")
	}
	token, err := r.openAgentTaskToolCapability(ctx, task.ID, sandbox.ID(),
		[]catalogdomain.ToolID{ids["workspace.read"], ids["workspace.edit"]})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.closeAgentTaskToolCapability(token) })
	return &governedTaskToolFixture{runtime: r, task: task, agent: agent, sandbox: sandbox,
		profile: profile, owner: owner, ids: ids, root: root, token: token, executor: exec}
}
func callTaskTool(t *testing.T, f *governedTaskToolFixture, key string, raw string) invocationdomain.Outcome {
	t.Helper()
	result, err := f.runtime.invokeAgentTaskTool(context.Background(), f.token, f.ids[key], []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return result.Outcome
}
func TestAgentTaskToolGatewayUsesCanonicalInvocationAndRetainsToolIDs(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	read := callTaskTool(t, f, "workspace.read", `{"path":"review.txt"}`)
	if read.Kind() != invocationdomain.OutcomeKindComplete || !strings.Contains(string(read.Content().Bytes()), "governed temporary contents") {
		t.Fatalf("canonical Tool read failed: %+v", read)
	}
	edit := callTaskTool(t, f, "workspace.edit", `{"path":"new-file.txt","operation":"create","content":"governed new contents\n"}`)
	if edit.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("canonical edit %+v", edit)
	}
	created, err := os.ReadFile(filepath.Join(f.root, "new-file.txt"))
	if err != nil || string(created) != "governed new contents\n" {
		t.Fatalf("expected disposable edit, %s %v", created, err)
	}
	// A shell operation is never allowed by the gateway even when the upstream
	// Profile is bound to canonical workspace.shell.
	if _, err := f.runtime.invokeAgentTaskTool(context.Background(), f.token, f.ids["workspace.shell"],
		[]byte(`{"command":"touch marker"}`)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("shell escaped task Tool allowlist: %v", err)
	}
	after, err := f.runtime.SandboxWorkspaceTools(context.Background(), f.sandbox.ID(), f.owner)
	if err != nil || after.ProviderID == "" || after.Status != "ready" {
		t.Fatalf("Suite changed %+v %v", after, err)
	}
	for _, tool := range after.Tools {
		if f.ids[tool.SourceKey].String() != tool.ToolID {
			t.Fatal("ToolID rotated")
		}
	}
	// The gateway never alters Profile ToolBindings by issuing a Task token.
	present, err := f.runtime.Profiles.GetByID.Execute(context.Background(), f.profile.ID())
	if err != nil || len(present.Bindings()) != 3 {
		t.Fatalf("Profile bindings changed: %v %v", present.Bindings(), err)
	}
}
func TestAgentTaskToolGatewayRejectsRevokedAgentGrantAndExpiredToken(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	if err := f.runtime.Agents.Revoke(context.Background(), f.agent.ID(), f.profile.ID().String(), f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.invokeAgentTaskTool(context.Background(), f.token, f.ids["workspace.edit"],
		[]byte(`{"path":"forbidden.txt","operation":"create","content":"must-not-appear"}`)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("revoked grant used: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "forbidden.txt")); !os.IsNotExist(err) {
		t.Fatal("denied edit modified filesystem")
	}
	f.runtime.closeAgentTaskToolCapability(f.token)
	if _, err := f.runtime.invokeAgentTaskTool(context.Background(), f.token, f.ids["workspace.read"],
		[]byte(`{"path":"review.txt"}`)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("closed capability replayed: %v", err)
	}
}
func TestAgentTaskToolGatewayRejectsProfileBindingRevocation(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	if _, err := f.runtime.Profiles.RemoveTool.Execute(context.Background(), profileapp.RemoveToolRequest{
		ProfileSlug: f.profile.Slug(), ToolID: f.ids["workspace.edit"],
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.invokeAgentTaskTool(context.Background(), f.token, f.ids["workspace.edit"],
		[]byte(`{"path":"forbidden.txt","operation":"create","content":"denied"}`)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("removed binding used: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "forbidden.txt")); !os.IsNotExist(err) {
		t.Fatal("removed Tool binding modified filesystem")
	}
}
func TestAgentTaskToolGatewayNoProfileSelfDelegation(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	if _, err := f.runtime.openAgentTaskToolCapability(context.Background(), f.task.ID, f.sandbox.ID(),
		[]catalogdomain.ToolID{f.ids["workspace.shell"]}); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("task gateway opened native shell: %v", err)
	}
	fakeToken := strings.Repeat("b", 64)
	if _, err := f.runtime.invokeAgentTaskTool(context.Background(), fakeToken, f.ids["workspace.read"],
		[]byte(`{"path":"review.txt"}`)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("unauthorized capability accepted: %v", err)
	}
	// Workspace tools are not Agent Delegation: same gateway rejects a different
	// canonical ToolID even if a Profile grants that Agent other Tools.
}
func TestAgentTaskToolGatewayCancellingTaskDeniesAndCannotMutateFiles(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	if err := f.runtime.Agents.Cancel(context.Background(), f.task.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.invokeAgentTaskTool(context.Background(), f.token, f.ids["workspace.edit"],
		[]byte(`{"path":"after-cancel.txt","operation":"create","content":"denied"}`)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("cancelled Agent Task escaped tool gateway: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "after-cancel.txt")); !os.IsNotExist(err) {
		t.Fatalf("cancelled Task mutated filesystem: %v", err)
	}
}
