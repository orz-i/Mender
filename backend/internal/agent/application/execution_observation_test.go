package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type observedFixtureExecutor struct {
	failStage agentdomain.ExecutionEventKind
}

func (e observedFixtureExecutor) ExecuteConfigured(_ context.Context, _ agentdomain.Agent, _ string, c RunConfig) (string, error) {
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 70001},
		{Kind: agentdomain.EventSessionReady, NativeSession: "native-private-session", CanLoad: true},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventProgress},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if event.Kind == e.failStage && event.Kind == agentdomain.EventSessionReady {
			event.NativeSession = strings.Repeat("x", 513)
		}
		if err := c.Observe(event); err != nil {
			if event.Kind == agentdomain.EventPromptSent {
				return "", fmt.Errorf("%w: synthetic prompt evidence failure", agentdomain.ErrOutcomeUncertain)
			}
			return "", err
		}
		if event.Kind == e.failStage && event.Kind == agentdomain.EventPromptSent {
			return "", fmt.Errorf("%w: synthetic post-prompt failure", agentdomain.ErrOutcomeUncertain)
		}
	}
	return "fixture completed", nil
}

func TestServiceTaskObservationContainsNoNativeIdentifiers(t *testing.T) {
	s, o := newFixtureService(t, observedFixtureExecutor{})
	a, err := s.Create(context.Background(), CreateRequest{
		Name: "Observed OpenCode", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: t.TempDir(),
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(context.Background(), a.ID(), "fixture prompt", o)
	if err != nil {
		t.Fatal(err)
	}
	got := waitTask(t, s, task.ID, o, agentdomain.TaskCompleted)
	if got.Observation == nil || got.Observation.Phase != "leader_reaped" ||
		!got.Observation.SessionRecorded || !got.Observation.LeaderReaped ||
		got.Observation.SessionRecoverability != "unverified" || got.Observation.LastObservedAt.IsZero() {
		t.Fatalf("missing safe observation %+v", got)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"native-private-session", "70001", a.Spec().WorkspaceRoot} {
		if strings.Contains(string(b), sensitive) {
			t.Fatalf("native execution metadata leaked in Task projection: %s", sensitive)
		}
	}
	items, err := s.Tasks(context.Background(), a.ID(), o)
	if err != nil || len(items) != 1 || items[0].Observation == nil {
		t.Fatalf("owner Task list lost evidence: %+v %v", items, err)
	}
}

func TestServiceRejectsUnpersistedSessionBeforeSendingPrompt(t *testing.T) {
	s, o := newFixtureService(t, observedFixtureExecutor{failStage: agentdomain.EventSessionReady})
	a, err := s.Create(context.Background(), CreateRequest{
		Name: "Fail closed", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: t.TempDir(),
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(context.Background(), a.ID(), "fixture prompt", o)
	if err != nil {
		t.Fatal(err)
	}
	got := waitTask(t, s, task.ID, o, agentdomain.TaskFailed)
	if got.Observation == nil || got.Observation.SessionRecorded || got.Observation.Phase != "process_started" {
		t.Fatalf("failed native persistence should not dispatch: %+v", got)
	}
}

func TestServiceMarksUncertainPostDispatchEvidenceFailureUnknown(t *testing.T) {
	s, o := newFixtureService(t, observedFixtureExecutor{failStage: agentdomain.EventPromptSent})
	a, err := s.Create(context.Background(), CreateRequest{
		Name: "Evidence loss", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: t.TempDir(),
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(context.Background(), a.ID(), "fixture prompt", o)
	if err != nil {
		t.Fatal(err)
	}
	got := waitTask(t, s, task.ID, o, agentdomain.TaskUnknown)
	if got.Diagnostic != "Agent outcome unknown after execution evidence loss" ||
		got.Result != "" || got.Observation == nil || got.Observation.Phase != "prompt_sent" {
		t.Fatalf("post-dispatch uncertainty understated: %+v", got)
	}
	if err := s.Cancel(context.Background(), task.ID, o); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("unknown original Task was cancellable: %v", err)
	}
}
