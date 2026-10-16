package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

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
	agent, err := s.Create(ctx, CreateRequest{Name: "Reusable", Backend: agentdomain.BackendCodex, WorkspaceRoot: root, DefaultModel: "example-model"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	sandboxID := "sbx_0123456789abcdef0123456789abcdef"
	if err := s.SetWorkspaceResolver(func(_ context.Context, id string) (WorkspaceTarget, error) {
		return WorkspaceTarget{ID: id, Root: other, Backend: "local_workspace", Isolation: "restricted_host", OwnerKind: string(caller.Principal().Kind()), OwnerRef: caller.Principal().Reference()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.BindWorkspace(ctx, agent.ID(), "project-b", sandboxID, caller); err != nil {
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
	task, err := s.SubmitWithOptions(ctx, agent.ID(), "review", SubmitOptions{Workspace: "project-b", Model: "other-model"}, remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeWorkspace(ctx, agent.ID(), "project-b", goodProfileID, caller); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDefaultModel(ctx, agent.ID(), "new-default", caller); err != nil {
		t.Fatal(err)
	}
	select {
	case selection := <-runner.calls:
		if selection.WorkspaceRoot != other || selection.Model != "other-model" {
			t.Fatalf("admitted snapshot changed: %+v", selection)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not run")
	}
	persisted, err := s.Task(ctx, task.ID, caller)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Model != "other-model" || persisted.WorkspaceKey != "project-b" || persisted.WorkspaceRoot != other {
		t.Fatalf("snapshot lost: %+v", persisted)
	}
	if _, err := s.SubmitWithOptions(ctx, agent.ID(), "again", SubmitOptions{Workspace: "project-b"}, remote); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("revoked workspace admitted: %v", err)
	}
}
