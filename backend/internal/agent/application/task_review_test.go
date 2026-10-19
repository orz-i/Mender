package application

import (
	"context"
	"errors"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestOwnerReportsUnknownHandledElsewhereWithoutReplayingOrRewriting(t *testing.T) {
	ctx := context.Background()
	s, owner := newFixtureService(t, observedFixtureExecutor{failStage: agentdomain.EventPromptSent})
	agent, err := s.Create(ctx, CreateRequest{Name: "Handled elsewhere", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: t.TempDir(), Isolation: "host"}, owner)
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := s.Submit(ctx, agent.ID(), "original work", owner)
	if err != nil {
		t.Fatal(err)
	}
	before := waitTask(t, s, submitted.ID, owner, agentdomain.TaskUnknown)
	if before.ResumeAvailability == nil || before.ResumeAvailability.Eligible || before.ResumeAvailability.Reason != "unknown_outcome" {
		t.Fatalf("historical unknown gained resume eligibility %+v", before.ResumeAvailability)
	}
	if before.Disposition != nil {
		t.Fatalf("unknown Task auto-annotated %+v", before.Disposition)
	}
	handled, err := s.ReviewUnknownTask(ctx, before.ID, agentdomain.UnknownHandledElsewhere, owner)
	if err != nil {
		t.Fatal(err)
	}
	if handled.Status != agentdomain.TaskUnknown || handled.Result != "" || handled.Diagnostic != before.Diagnostic ||
		handled.Disposition == nil || handled.Disposition.Handling != agentdomain.UnknownHandledElsewhere ||
		handled.ResumeAvailability == nil || handled.ResumeAvailability.Eligible {
		t.Fatalf("original unknown result rewritten %+v", handled)
	}
	if _, err := s.Resume(ctx, handled.ID, "DO NOT REPLAY", agent.ID(), owner); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("acknowledged unknown was resumed: %v", err)
	}
	if _, err := s.ReviewUnknownTask(ctx, handled.ID, "mark_complete", owner); !errors.Is(err, agentdomain.ErrInvalid) {
		t.Fatalf("arbitrary owner disposition accepted: %v", err)
	}
	restored, err := s.ReviewUnknownTask(ctx, before.ID, agentdomain.UnknownNeedsReview, owner)
	if err != nil || restored.Status != agentdomain.TaskUnknown || restored.Disposition == nil ||
		restored.Disposition.Handling != agentdomain.UnknownNeedsReview {
		t.Fatalf("reopened review changed original task %+v %v", restored, err)
	}
	tasks, err := s.Tasks(ctx, agent.ID(), owner)
	if err != nil || len(tasks) != 1 || tasks[0].Disposition == nil || tasks[0].Status != agentdomain.TaskUnknown {
		t.Fatalf("Task owner listing lost review %+v %v", tasks, err)
	}
}

func TestUnknownTaskDispositionOwnerOnlyAndEligiblePreviewIsNotAuthorization(t *testing.T) {
	ctx := context.Background()
	s, owner := newFixtureService(t, &nativeResumeFixtureExecutor{})
	agent, err := s.Create(ctx, CreateRequest{Name: "Owner ACP continuation",
		Backend: agentdomain.BackendOpenCode, WorkspaceRoot: t.TempDir(), Isolation: "host"}, owner)
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := s.Submit(ctx, agent.ID(), "synthetic", owner)
	if err != nil {
		t.Fatal(err)
	}
	got := waitTask(t, s, submitted.ID, owner, agentdomain.TaskCompleted)
	if got.ResumeAvailability == nil || !got.ResumeAvailability.Eligible || got.ResumeAvailability.Reason != "candidate" {
		t.Fatalf("owner eligible preview missing %+v", got.ResumeAvailability)
	}
	if _, err := s.ReviewUnknownTask(ctx, got.ID, agentdomain.UnknownHandledElsewhere, owner); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("completed Task disposition allowed: %v", err)
	}
	if _, err := s.ReviewUnknownTask(ctx, got.ID, agentdomain.UnknownHandledElsewhere, profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile caller edited owner Task review %v", err)
	}
}
