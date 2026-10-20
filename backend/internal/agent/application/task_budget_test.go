package application

import (
	"context"
	"errors"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type boundedDeadlineRunner struct{ remaining chan time.Duration }

func (r *boundedDeadlineRunner) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, cfg RunConfig) (string, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return "", errors.New("unbounded Agent Task context")
	}
	r.remaining <- time.Until(deadline)
	if err := observeFixtureTerminal(cfg); err != nil {
		return "", err
	}
	return "ok", nil
}

func TestSubmittedTaskBudgetIsBoundedAndImmutable(t *testing.T) {
	ctx := context.Background()
	runner := &boundedDeadlineRunner{remaining: make(chan time.Duration, 4)}
	s, owner := newFixtureService(t, runner)
	agent, err := s.Create(ctx, CreateRequest{
		Name: "Budgeted Agent", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}

	for _, invalid := range []int{-10, -1, 1, 9, 11, 31, 1000000000} {
		if _, err := s.SubmitWithOptions(ctx, agent.ID(), "do safe task", SubmitOptions{
			TimeoutMinutes: invalid,
		}, owner); !errors.Is(err, agentdomain.ErrInvalid) {
			t.Fatalf("accepted invalid execution budget %d: %v", invalid, err)
		}
	}
	items, err := s.Tasks(ctx, agent.ID(), owner)
	if err != nil || len(items) != 0 {
		t.Fatalf("invalid budget created Tasks %+v %v", items, err)
	}

	for _, tc := range []struct{ minutes, wantSeconds int }{
		{0, 600}, {10, 600}, {20, 1200}, {30, 1800},
	} {
		task, err := s.SubmitWithOptions(ctx, agent.ID(), "fixture no native tools", SubmitOptions{
			TimeoutMinutes: tc.minutes,
		}, owner)
		if err != nil {
			t.Fatalf("admit %d minutes: %v", tc.minutes, err)
		}
		if task.TimeoutSeconds != tc.wantSeconds || task.ExecutionTimeout().Seconds() != float64(tc.wantSeconds) {
			t.Fatalf("Task budget not snapshotted: %+v", task)
		}
		select {
		case left := <-runner.remaining:
			if left <= time.Duration(tc.wantSeconds-3)*time.Second || left > time.Duration(tc.wantSeconds)*time.Second {
				t.Fatalf("runner deadline %s does not match admitted %d seconds", left, tc.wantSeconds)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("runner did not receive admitted task")
		}
		stored := waitTask(t, s, task.ID, owner, agentdomain.TaskCompleted)
		if stored.TimeoutSeconds != tc.wantSeconds {
			t.Fatalf("Task budget lost on reload: %v", stored.TimeoutSeconds)
		}
		if stored.Result != "" && stored.Result != "ok" {
			t.Fatalf("unexpected result: %q", stored.Result)
		}
	}
}
