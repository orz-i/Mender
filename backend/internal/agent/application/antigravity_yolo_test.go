package application

import (
	"context"
	"errors"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type nativePermissionSnapshot struct {
	agent  agentdomain.Backend
	config RunConfig
}
type nativeSnapshotRunner struct{ seen chan nativePermissionSnapshot }

func (r nativeSnapshotRunner) ExecuteConfigured(_ context.Context, a agentdomain.Agent, _ string, cfg RunConfig) (string, error) {
	r.seen <- nativePermissionSnapshot{agent: a.Spec().Backend, config: cfg}
	return "fixture", nil
}

func TestAntigravityYOLOOnlyFromOwnerTrustedUnexpiredMatchingWorkspace(t *testing.T) {
	ctx := context.Background()
	runner := nativeSnapshotRunner{seen: make(chan nativePermissionSnapshot, 8)}
	s, owner := newFixtureService(t, runner)
	root, second := t.TempDir(), t.TempDir()
	a, err := s.Create(ctx, CreateRequest{Name: "AGY fixture", Backend: agentdomain.BackendAntigravity, WorkspaceRoot: root}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindWorkspace(ctx, a.ID(), "second", second, owner); err != nil {
		t.Fatal(err)
	}
	run := func(workspace string, wantYOLO bool) {
		t.Helper()
		task, err := s.SubmitWithOptions(ctx, a.ID(), "fixture prompt", SubmitOptions{Workspace: workspace}, owner)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-runner.seen:
			if got.agent != agentdomain.BackendAntigravity || got.config.AllowNativeYOLO != wantYOLO {
				t.Fatalf("workspace=%s gotYOLO=%v want=%v", workspace, got.config.AllowNativeYOLO, wantYOLO)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no process launch snapshot")
		}
		_ = waitTask(t, s, task.ID, owner, agentdomain.TaskCompleted)
	}
	run("", false)
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalWorkspaceRead, owner); !errors.Is(err, agentdomain.ErrUnavailable) {
		t.Fatalf("unreviewable Antigravity workspace_read accepted: %v", err)
	}
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalTrusted, profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile could set YOLO: %v", err)
	}
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalTrusted, owner); err != nil {
		t.Fatal(err)
	}
	run("", true)
	run("second", false) // trust is scoped to exact workspace key
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalManual, owner); err != nil {
		t.Fatal(err)
	}
	run("", false)
	// Existing table retention/mode lookup must never re-enable an expired flag.
	p := agentdomain.ApprovalPolicy{AgentID: a.ID(), WorkspaceRoot: a.Spec().WorkspaceRoot, Mode: agentdomain.ApprovalTrusted, UpdatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(120 * time.Millisecond).UTC()}
	if err := s.store.SetApprovalPolicy(ctx, p); err != nil {
		t.Fatal(err)
	}
	time.Sleep(160 * time.Millisecond)
	run("", false)
}
