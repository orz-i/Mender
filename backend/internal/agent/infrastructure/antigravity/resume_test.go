package antigravity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

const fixtureConversationID = "dfe3c2a6-211b-48dc-bfa8-a5d2ccf51b85"

func resumeFixturePlan(t *testing.T, a agentdomain.Agent) *agentdomain.SessionContinuation {
	t.Helper()
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	return &agentdomain.SessionContinuation{
		SourceTaskID: taskID, SessionRef: "asess_" + strings.Repeat("f", 32),
		NativeSessionID: fixtureConversationID, AgentID: a.ID(),
		Backend: agentdomain.BackendAntigravity, WorkspaceRoot: a.Spec().WorkspaceRoot,
		Isolation: "host", PrincipalKind: "local_user", PrincipalRef: "os:test", ScopeKind: "catalog",
		CanLoad: true, CanResume: true,
	}
}

func TestHeadlessContinuationChecksNativeIdentityBeforeDispatch(t *testing.T) {
	script := scriptFixture(t, `printf '%s\n' '{"event":"init","conversation_id":"`+fixtureConversationID+`"}'
IFS= read -r line || exit 21
case "$line" in *"NEW_PROMPT_ONLY"*) ;; *) exit 22;; esac
printf '%s\n' '{"event":"step_update"}' '{"event":"result","conversation_id":"`+fixtureConversationID+`","result":{"status":"SUCCESS","response":"resumed"}}'`)
	a := agyFixture(t)
	plan := resumeFixturePlan(t, a)
	calls := 0
	var seen []agentdomain.ExecutionEvent
	r := Runner{
		NewCommand: func(context.Context, agentdomain.Agent, string, string, string, bool, bool) (*exec.Cmd, error) {
			t.Fatal("resumption launched a new conversation without --conversation")
			return nil, nil
		},
		NewResumeCommand: func(ctx context.Context, _ agentdomain.Agent, root, model, effort, conversation string, yolo bool) (*exec.Cmd, error) {
			calls++
			if root != a.Spec().WorkspaceRoot || model != "" || effort != "" || conversation != fixtureConversationID || yolo {
				t.Fatalf("wrong native resume command parameters: root=%q cid=%q", root, conversation)
			}
			return exec.CommandContext(ctx, script), nil
		},
	}
	result, err := r.ExecuteConfigured(context.Background(), a, "NEW_PROMPT_ONLY", agentapp.RunConfig{
		WorkspaceRoot: a.Spec().WorkspaceRoot, Resume: plan,
		Observe: func(e agentdomain.ExecutionEvent) error { seen = append(seen, e); return nil },
	})
	if err != nil || result != "resumed" || calls != 1 {
		t.Fatalf("continue result=%q err=%v calls=%d", result, err, calls)
	}
	sequence := []agentdomain.ExecutionEventKind{
		agentdomain.EventProcessStarted, agentdomain.EventSessionReady,
		agentdomain.EventPromptSent, agentdomain.EventProgress,
		agentdomain.EventTerminalSeen, agentdomain.EventLeaderReaped,
	}
	if len(seen) != len(sequence) {
		t.Fatalf("continuation event ordering: %+v", seen)
	}
	for i, kind := range sequence {
		if seen[i].Kind != kind {
			t.Fatalf("event %d=%+v, want %s", i, seen[i], kind)
		}
	}
	if seen[1].NativeSession != fixtureConversationID || seen[1].ExistingSessionRef != plan.SessionRef ||
		!seen[1].CanLoad || !seen[1].CanResume {
		t.Fatalf("native identity was not verified+bound before dispatch: %+v", seen[1])
	}
}

func TestHeadlessContinuationNeverSendsPromptToNativeFallback(t *testing.T) {
	a := agyFixture(t)
	plan := resumeFixturePlan(t, a)
	for _, tc := range []struct{ name, firstLine string }{
		{"different_valid_uuid", `{"event":"init","conversation_id":"00000000-0000-4000-8000-000000000000"}`},
		{"missing_id", `{"event":"init"}`},
		{"preprompt_result", `{"event":"result","result":{"status":"SUCCESS","response":"fallback"}}`},
		{"malformed_init", `{"event":"init","conversation_id":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flag := filepath.Join(t.TempDir(), "prompt-was-sent")
			script := scriptFixture(t, fmt.Sprintf(`printf '%%s\n' '%s'
if IFS= read -r line; then printf sent > '%s'; fi`, tc.firstLine, flag))
			var events []agentdomain.ExecutionEventKind
			r := Runner{NewCommand: func(context.Context, agentdomain.Agent, string, string, string, bool, bool) (*exec.Cmd, error) {
				t.Fatal("incorrect new CLI path")
				return nil, nil
			}, NewResumeCommand: func(ctx context.Context, _ agentdomain.Agent, _, _, _, _ string, _ bool) (*exec.Cmd, error) {
				return exec.CommandContext(ctx, script), nil
			}}
			_, err := r.ExecuteConfigured(context.Background(), a, "MUST_NOT_BE_SENT", agentapp.RunConfig{
				WorkspaceRoot: a.Spec().WorkspaceRoot, Resume: plan,
				Observe: func(e agentdomain.ExecutionEvent) error { events = append(events, e.Kind); return nil },
			})
			if !errors.Is(err, agentdomain.ErrResumeUnavailable) {
				t.Fatalf("invalid/fallback identity accepted: %v", err)
			}
			var staged interface{ SafeAgentFailureStage() string }
			if !errors.As(err, &staged) || staged.SafeAgentFailureStage() != "headless_resume" {
				t.Fatalf("pre-prompt resume rejected without safe diagnostic: %v", err)
			}
			for _, kind := range events {
				if kind == agentdomain.EventPromptSent || kind == agentdomain.EventSessionReady {
					t.Fatalf("pre-prompt mismatch dispatched or bound identity: %v", events)
				}
			}
			if _, err = os.Stat(flag); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("native fallback was sent a prompt: %v", err)
			}
		})
	}
}

func TestPrePromptResumeUnavailableRetainsSentinelAndSafeStage(t *testing.T) {
	err := unavailableResumeBeforePrompt()
	if !errors.Is(err, agentdomain.ErrResumeUnavailable) || errors.Is(err, agentdomain.ErrOutcomeUncertain) {
		t.Fatalf("pre-prompt resume failure must not claim executed or uncertain task: %v", err)
	}
	var stage interface{ SafeAgentFailureStage() string }
	if !errors.As(err, &stage) || stage.SafeAgentFailureStage() != "headless_resume" {
		t.Fatalf("no fixed diagnostic stage for failed native identity proof: %v", err)
	}
}

func TestHeadlessContinuationRejectsUnverifiedInitTimeoutAndStorageFailure(t *testing.T) {
	a := agyFixture(t)
	plan := resumeFixturePlan(t, a)
	blocking := scriptFixture(t, `IFS= read -r line || exit 12
printf '%s\n' '{"event":"init","conversation_id":"`+fixtureConversationID+`"}'`)
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 140*time.Millisecond)
	defer cancel()
	var sent bool
	r := Runner{NewCommand: func(context.Context, agentdomain.Agent, string, string, string, bool, bool) (*exec.Cmd, error) {
		t.Fatal("new command used")
		return nil, nil
	}, NewResumeCommand: func(ctx context.Context, _ agentdomain.Agent, _, _, _, _ string, _ bool) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, blocking), nil
	}}
	_, err := r.ExecuteConfigured(timeoutCtx, a, "NEVER_SEND", agentapp.RunConfig{
		WorkspaceRoot: a.Spec().WorkspaceRoot, Resume: plan,
		Observe: func(e agentdomain.ExecutionEvent) error {
			if e.Kind == agentdomain.EventPromptSent {
				sent = true
			}
			return nil
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) || sent {
		t.Fatalf("missing preprompt init did not fail closed: err=%v sent=%v", err, sent)
	}
	ready := scriptFixture(t, `printf '%s\n' '{"event":"init","conversation_id":"`+fixtureConversationID+`"}'
IFS= read -r line || exit 11`)
	storageFailure := errors.New("synthetic Task evidence failure")
	r.NewResumeCommand = func(ctx context.Context, _ agentdomain.Agent, _, _, _, _ string, _ bool) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, ready), nil
	}
	sent = false
	_, err = r.ExecuteConfigured(context.Background(), a, "MUST_NOT_SEND", agentapp.RunConfig{
		WorkspaceRoot: a.Spec().WorkspaceRoot, Resume: plan,
		Observe: func(e agentdomain.ExecutionEvent) error {
			if e.Kind == agentdomain.EventPromptSent {
				sent = true
			}
			if e.Kind == agentdomain.EventSessionReady {
				return storageFailure
			}
			return nil
		},
	})
	if !errors.Is(err, storageFailure) || sent {
		t.Fatalf("pre-prompt native session persistence failure dispatched task: err=%v sent=%v", err, sent)
	}
}

func TestHeadlessContinuationRejectsPostPromptTerminalIdentityAsUnknown(t *testing.T) {
	a := agyFixture(t)
	plan := resumeFixturePlan(t, a)
	script := scriptFixture(t, `printf '%s\n' '{"event":"init","conversation_id":"`+fixtureConversationID+`"}'
IFS= read -r line || exit 11
printf '%s\n' '{"event":"result","conversation_id":"00000000-0000-4000-8000-000000000000","result":{"status":"SUCCESS","response":"wrong session"}}'`)
	r := Runner{NewCommand: func(context.Context, agentdomain.Agent, string, string, string, bool, bool) (*exec.Cmd, error) {
		t.Fatal("new command used")
		return nil, nil
	}, NewResumeCommand: func(ctx context.Context, _ agentdomain.Agent, _, _, _, _ string, _ bool) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, script), nil
	}}
	var dispatched bool
	_, err := r.ExecuteConfigured(context.Background(), a, "a sent prompt", agentapp.RunConfig{
		WorkspaceRoot: a.Spec().WorkspaceRoot, Resume: plan,
		Observe: func(e agentdomain.ExecutionEvent) error {
			if e.Kind == agentdomain.EventPromptSent {
				dispatched = true
			}
			return nil
		},
	})
	if !dispatched || !errors.Is(err, agentdomain.ErrOutcomeUncertain) {
		t.Fatalf("post-prompt terminal identity mismatch must remain unknown: sent=%v err=%v", dispatched, err)
	}
}
