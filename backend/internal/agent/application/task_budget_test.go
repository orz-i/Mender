package application

import (
	"context"
	"errors"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestSubmittedTaskBudgetIsBoundedAndImmutable(t *testing.T) {
	ctx := context.Background()
	s, owner := newFixtureService(t, &inspectingFake{})
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
		stored := waitTask(t, s, task.ID, owner, agentdomain.TaskCompleted)
		if stored.TimeoutSeconds != tc.wantSeconds {
			t.Fatalf("Task budget lost on reload: %v", stored.TimeoutSeconds)
		}
		if stored.Result != "" && stored.Result != "ok" {
			t.Fatalf("unexpected result: %q", stored.Result)
		}
	}
}
