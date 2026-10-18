package acp

import (
	"context"
	"errors"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestACPExecutionEvidenceEmittedBeforePromptAndNativeResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	a := fixtureAgent(t)
	var events []agentdomain.ExecutionEvent
	got, err := fixtureRunner("session-capabilities").ExecuteConfigured(ctx, a, "bounded prompt",
		agentapplication.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot,
			Observe: func(e agentdomain.ExecutionEvent) error {
				events = append(events, e)
				return nil
			}})
	if err != nil || got != "fixture completed" {
		t.Fatalf("prompt completion %q %v", got, err)
	}
	if len(events) < 6 {
		t.Fatalf("missing execution evidence: %+v", events)
	}
	if events[0].Kind != agentdomain.EventProcessStarted || events[0].ProcessPID <= 0 ||
		events[1].Kind != agentdomain.EventSessionReady ||
		events[1].NativeSession != "fixture-1" || !events[1].CanLoad || !events[1].CanResume ||
		events[2].Kind != agentdomain.EventPromptSent ||
		events[len(events)-2].Kind != agentdomain.EventTerminalSeen ||
		events[len(events)-1].Kind != agentdomain.EventLeaderReaped {
		t.Fatalf("incorrect ordered evidence %+v", events)
	}
}

func TestACPExecutionEvidenceFailureBeforePromptPreventsDispatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	a := fixtureAgent(t)
	var dispatched bool
	_, err := fixtureRunner("session-capabilities").ExecuteConfigured(ctx, a, "private prompt",
		agentapplication.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot,
			Observe: func(e agentdomain.ExecutionEvent) error {
				if e.Kind == agentdomain.EventSessionReady {
					return errors.New("synthetic SQLite failure before dispatch")
				}
				if e.Kind == agentdomain.EventPromptSent {
					dispatched = true
				}
				return nil
			}})
	if err == nil || dispatched {
		t.Fatalf("ACP continued after session write failure: dispatched=%v err=%v", dispatched, err)
	}
}

func TestACPDispatchedPromptEvidenceFailureIsUnknown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	a := fixtureAgent(t)
	var dispatched bool
	_, err := fixtureRunner("completed").ExecuteConfigured(ctx, a, "private prompt",
		agentapplication.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot,
			Observe: func(e agentdomain.ExecutionEvent) error {
				if e.Kind == agentdomain.EventPromptSent {
					dispatched = true
					return errors.New("synthetic commit loss")
				}
				return nil
			}})
	if !dispatched || !errors.Is(err, agentdomain.ErrOutcomeUncertain) {
		t.Fatalf("lost post-write ACK classified incorrectly: dispatched=%v err=%v", dispatched, err)
	}
}

func TestACPPrePromptRecoveryObservesTwoDistinctProcesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	a := fixtureAgent(t)
	var started, reaped, prompted int
	_, err := fixtureRunner("choice-always-model-missing").ExecuteConfigured(ctx, a, "private prompt",
		agentapplication.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot, Model: "fixture-two",
			Observe: func(e agentdomain.ExecutionEvent) error {
				switch e.Kind {
				case agentdomain.EventProcessStarted:
					started++
				case agentdomain.EventLeaderReaped:
					reaped++
				case agentdomain.EventPromptSent:
					prompted++
				}
				return nil
			}})
	if err == nil || started != 2 || reaped != 2 || prompted != 0 {
		t.Fatalf("ACP process setup recovery started=%d reaped=%d prompt=%d err=%v",
			started, reaped, prompted, err)
	}
}
