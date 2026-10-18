package antigravity

import (
	"context"
	"errors"
	"testing"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestHeadlessEvidenceCapturesExactConversationWithoutRawUpdates(t *testing.T) {
	script := scriptFixture(t, `IFS= read -r prompt || exit 12
printf '%s\n' '{"event":"init","conversation_id":"exact-conversation-123","init":{"cwd":"/private"}}' '{"event":"step_update","step_update":{"rawInput":{"password":"SECRET"}}}' '{"event":"result","result":{"status":"SUCCESS","response":"done"}}'`)
	a := agyFixture(t)
	var calls int
	var events []agentdomain.ExecutionEvent
	result, err := runnerFixture(script, &calls).ExecuteConfigured(context.Background(), a, "prompt",
		agentapplication.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot,
			Observe: func(e agentdomain.ExecutionEvent) error {
				events = append(events, e)
				return nil
			}})
	if err != nil || result != "done" || calls != 1 || len(events) < 6 {
		t.Fatalf("Headless evidence %q %v calls=%d events=%+v", result, err, calls, events)
	}
	if events[0].Kind != agentdomain.EventProcessStarted ||
		events[1].Kind != agentdomain.EventPromptSent ||
		events[2].Kind != agentdomain.EventSessionReady ||
		events[2].NativeSession != "exact-conversation-123" ||
		events[len(events)-2].Kind != agentdomain.EventTerminalSeen ||
		events[len(events)-1].Kind != agentdomain.EventLeaderReaped {
		t.Fatalf("Headless event order %+v", events)
	}
	for _, e := range events {
		if e.NativeSession != "" && e.NativeSession != "exact-conversation-123" {
			t.Fatalf("untrusted Headless payload entered evidence %+v", e)
		}
	}
}

func TestHeadlessSessionStorageFailureAfterDispatchIsUnknown(t *testing.T) {
	script := scriptFixture(t, `IFS= read -r prompt || exit 12
printf '%s\n' '{"event":"init","conversation_id":"exact-conversation-123"}' '{"event":"result","result":{"status":"SUCCESS","response":"done"}}'`)
	a := agyFixture(t)
	var calls int
	var sawPrompt bool
	result, err := runnerFixture(script, &calls).ExecuteConfigured(context.Background(), a, "prompt",
		agentapplication.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot,
			Observe: func(e agentdomain.ExecutionEvent) error {
				if e.Kind == agentdomain.EventPromptSent {
					sawPrompt = true
				}
				if e.Kind == agentdomain.EventSessionReady {
					return errors.New("synthetic SQLite failure")
				}
				return nil
			}})
	if result != "" || !sawPrompt || !errors.Is(err, agentdomain.ErrOutcomeUncertain) {
		t.Fatalf("post-prompt storage failure should remain unknown: %q %v %t", result, err, sawPrompt)
	}
}
