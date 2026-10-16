package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type inspectingFake struct{ probes int }

func (f *inspectingFake) Execute(context.Context, agentdomain.Agent, string) (string, error) {
	return "", nil
}
func (f *inspectingFake) InspectModels(context.Context, agentdomain.Agent) (ModelChoices, error) {
	f.probes++
	return ModelChoices{Current: "fixture-one", Models: []ModelChoice{{ID: "fixture-one", Name: "Fixture One"}}}, nil
}
func TestAgentModelInspectionRequiresOwnerAndNoTask(t *testing.T) {
	executor := &inspectingFake{}
	service, caller := newFixtureService(t, executor)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agent, err := service.Create(context.Background(), CreateRequest{
		Name: "Model Test", Backend: agentdomain.BackendGrok, WorkspaceRoot: root,
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.InspectModels(context.Background(), agent.ID(), profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("profile inspector was allowed: %v", err)
	}
	if executor.probes != 0 {
		t.Fatal("untrusted Profile launched ACP probe")
	}
	models, err := service.InspectModels(context.Background(), agent.ID(), caller)
	if err != nil || len(models.Models) != 1 || models.Current != "fixture-one" {
		t.Fatalf("choices=%+v err=%v", models, err)
	}
	if executor.probes != 1 {
		t.Fatalf("unexpected probe calls %d", executor.probes)
	}
	tasks, err := service.Tasks(context.Background(), agent.ID(), caller)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("inspection created task: %+v %v", tasks, err)
	}
}

type selectionRunner struct{ calls chan RunConfig }

func (r selectionRunner) Execute(context.Context, agentdomain.Agent, string) (string, error) {
	return "", errors.New("unconfigured execution is forbidden")
}
func (r selectionRunner) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, config RunConfig) (string, error) {
	select {
	case r.calls <- config:
		return "ok", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func TestRuntimeSelectionsRequireExplicitWorkspaceGrantAndSnapshot(t *testing.T) {
	ctx := context.Background()
	store, err := agentsqlite.Open(ctx, filepath.Join(t.TempDir(), "selections.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := selectionRunner{calls: make(chan RunConfig, 4)}
	caller := owner(t)
	s, err := NewService(store, runner, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil },
		caller.Principal(), func(_ context.Context, id string) error {
			if id == goodProfileID {
				return nil
			}
			return errors.New("no profile")
		})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	agent, err := s.Create(ctx, CreateRequest{Name: "Reusable", Backend: agentdomain.BackendCodex, WorkspaceRoot: root, DefaultModel: "example-model", DefaultReasoningEffort: "medium"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	// Adding a local Agent cwd requires no Sandbox resource or router.
	if err := s.BindWorkspace(ctx, agent.ID(), "missing", filepath.Join(other, "not-found"), caller); !errors.Is(err, agentdomain.ErrInvalid) {
		t.Fatalf("nonexistent directory accepted: %v", err)
	}
	if _, err := s.SubmitWithOptions(ctx, agent.ID(), "review", SubmitOptions{ReasoningEffort: "XHIGH"}, caller); !errors.Is(err, agentdomain.ErrInvalid) {
		t.Fatalf("unsafe effort accepted: %v", err)
	}
	select {
	case <-runner.calls:
		t.Fatal("unsafe effort launched Agent")
	default:
	}
	if err := s.BindWorkspace(ctx, agent.ID(), "project-b", other, caller); err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, agent.ID(), goodProfileID, caller); err != nil {
		t.Fatal(err)
	}
	remote := profile(t, goodProfileID)
	if _, err := s.SubmitWithOptions(ctx, agent.ID(), "review", SubmitOptions{Workspace: "project-b", Model: "example-model"}, remote); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("ungranted workspace accepted: %v", err)
	}
	select {
	case <-runner.calls:
		t.Fatal("executed ungranted workspace")
	default:
	}
	if err := s.GrantWorkspace(ctx, agent.ID(), "project-b", goodProfileID, caller); err != nil {
		t.Fatal(err)
	}
	task, err := s.SubmitWithOptions(ctx, agent.ID(), "review", SubmitOptions{Workspace: "project-b", Model: "other-model", ReasoningEffort: "xhigh"}, remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeWorkspace(ctx, agent.ID(), "project-b", goodProfileID, caller); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDefaults(ctx, agent.ID(), "new-default", "high", caller); err != nil {
		t.Fatal(err)
	}
	select {
	case selection := <-runner.calls:
		if selection.WorkspaceRoot != other || selection.Model != "other-model" || selection.ReasoningEffort != "xhigh" {
			t.Fatalf("admitted snapshot changed: %+v", selection)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not run")
	}
	persisted, err := s.Task(ctx, task.ID, caller)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Model != "other-model" || persisted.ReasoningEffort != "xhigh" || persisted.WorkspaceKey != "project-b" || persisted.WorkspaceRoot != other {
		t.Fatalf("snapshot lost: %+v", persisted)
	}
	defaultTask, err := s.Submit(ctx, agent.ID(), "using defaults", caller)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case config := <-runner.calls:
		if config.Model != "new-default" || config.ReasoningEffort != "high" || config.WorkspaceRoot != root {
			t.Fatalf("defaults not resolved together: %+v", config)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("default runner did not start")
	}
	storedDefault, err := s.Task(ctx, defaultTask.ID, caller)
	if err != nil || storedDefault.Model != "new-default" || storedDefault.ReasoningEffort != "high" {
		t.Fatalf("wrong persisted default snapshot %+v err=%v", storedDefault, err)
	}
	if _, err := s.SubmitWithOptions(ctx, agent.ID(), "again", SubmitOptions{Workspace: "project-b"}, remote); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("revoked workspace admitted: %v", err)
	}
}

func TestLocalAgentWorkspaceApprovalRejectsPathRedirectionWithoutLaunching(t *testing.T) {
	ctx := context.Background()
	store, err := agentsqlite.Open(ctx, filepath.Join(t.TempDir(), "local-agent-workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	runner := selectionRunner{calls: make(chan RunConfig, 2)}
	caller := owner(t)
	service, err := NewService(store, runner,
		func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil },
		caller.Principal(), func(_ context.Context, id string) error {
			if id == goodProfileID {
				return nil
			}
			return errors.New("unknown Profile")
		})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	defaultRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agent, err := service.Create(ctx, CreateRequest{Name: "Local direct workspaces", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: defaultRoot, Isolation: "host"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := filepath.EvalSymlinks(filepath.Join(t.TempDir(), "approved"))
	if err == nil {
		t.Fatal("unexpectedly resolved nonexistent target", approved)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	approved = filepath.Join(parent, "approved")
	if err := os.Mkdir(approved, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "short-link")
	if err := os.Symlink(approved, alias); err != nil {
		t.Fatal(err)
	}
	if err := service.BindWorkspace(ctx, agent.ID(), "project-b", alias, profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile could register arbitrary host cwd: %v", err)
	}
	if err := service.BindWorkspace(ctx, agent.ID(), "project-b", alias, caller); err != nil {
		t.Fatal(err)
	}
	bindings, err := service.Workspaces(ctx, agent.ID(), caller)
	if err != nil || len(bindings) != 1 || bindings[0].WorkspaceRoot != approved {
		t.Fatalf("owner-registered canonical root not persisted %+v %v", bindings, err)
	}
	if err := service.Grant(ctx, agent.ID(), goodProfileID, caller); err != nil {
		t.Fatal(err)
	}
	trustedProfile := profile(t, goodProfileID)
	if _, err := service.SubmitWithOptions(ctx, agent.ID(), "safe read",
		SubmitOptions{Workspace: "project-b"}, trustedProfile); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("ungranted profile selected extra workspace: %v", err)
	}
	if err := service.GrantWorkspace(ctx, agent.ID(), "project-b", goodProfileID, caller); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitWithOptions(ctx, agent.ID(), "safe read",
		SubmitOptions{Workspace: "project-b"}, trustedProfile); err != nil {
		t.Fatal(err)
	}
	select {
	case actual := <-runner.calls:
		if actual.WorkspaceRoot != approved {
			t.Fatalf("unexpected admitted cwd: %+v", actual)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("approved path never reached executor")
	}
	// A canonical registered directory replaced by a symlink must not gain
	// access to an arbitrary new target under the existing Profile grant.
	if err := os.Remove(approved); err != nil {
		t.Fatal(err)
	}
	replacement := t.TempDir()
	if err := os.Symlink(replacement, approved); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitWithOptions(ctx, agent.ID(), "MUST NOT EXECUTE",
		SubmitOptions{Workspace: "project-b"}, trustedProfile); !errors.Is(err, agentdomain.ErrUnavailable) {
		t.Fatalf("changed directory escaped selection review: %v", err)
	}
	select {
	case got := <-runner.calls:
		t.Fatalf("unapproved workspace launched: %+v", got)
	default:
	}
}
