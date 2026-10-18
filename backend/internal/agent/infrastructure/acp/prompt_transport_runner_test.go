package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
)

func TestACPPromptTransportTerminalReasonsFromRealSubprocess(t *testing.T) {
	for _, tc := range []struct {
		name, mode, stage string
		cause             error
	}{
		{"normal stdout EOF before response", "close-prompt-early", stagePromptStreamClosed, errACPStreamClosed},
		{"unparseable ACP JSON frame", "invalid-json", stagePromptMalformedFrame, errACPInvalidFrame},
		{"bounded scanner cannot read oversized frame", "oversized-acp-frame", stagePromptReadFailed, errACPReadFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			agent := fixtureAgent(t)
			got, err := fixtureRunner(tc.mode).ExecuteConfigured(ctx, agent, "synthetic prompt failure",
				agentapplication.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot})
			if err == nil || got != "" || !errors.Is(err, tc.cause) {
				t.Fatalf("ACP result=%q err=%v did not retain expected safe terminal cause", got, err)
			}
			var classified interface{ SafeAgentFailureStage() string }
			if !errors.As(err, &classified) || classified.SafeAgentFailureStage() != tc.stage {
				t.Fatalf("ACP class %q err=%v; want %q", func() string {
					if classified == nil {
						return "none"
					}
					return classified.SafeAgentFailureStage()
				}(), err, tc.stage)
			}
			if strings.Contains(err.Error(), "synthetic prompt") || strings.Contains(err.Error(), "Users/") {
				t.Fatal("an ACP failure persisted unsafe message content")
			}
		})
	}
}

// The output writer fails before any response can be read. No process or
// network access is required to prove the fixed write category.
type simulatedACPWriteFailure struct{}

func (simulatedACPWriteFailure) Write([]byte) (int, error) {
	return 0, fmt.Errorf("synthetic write failure with local-only private content")
}

func TestACPPromptWriteFailurePreservesSafeCategory(t *testing.T) {
	wire := &stream{stdin: simulatedACPWriteFailure{}}
	_, err := wire.call(context.Background(), 3, "session/prompt", map[string]any{
		"sessionId": "fixture-1", "prompt": []any{},
	}, func(rpcMessage) error { return nil })
	if err == nil || !errors.Is(err, errACPWriteFailed) {
		t.Fatalf("lost ACP write failure marker: %v", err)
	}
	failure := stagePromptError(err)
	var staged interface{ SafeAgentFailureStage() string }
	if !errors.As(failure, &staged) || staged.SafeAgentFailureStage() != stagePromptWriteFailed {
		t.Fatalf("unclassified ACP write failure: %v", failure)
	}
	if failure.Error() != "Agent task failed at stage "+stagePromptWriteFailed ||
		strings.Contains(failure.Error(), "synthetic write") {
		t.Fatalf("unsafe ACP write failure was exposed to Task results: %q", failure.Error())
	}
}

func TestACPPromptTransportFailureDoesNotOverrideCancelledContext(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		err := stagePromptError(fmt.Errorf("%w: %w", errACPStreamClosed, cause))
		if !errors.Is(err, cause) {
			t.Fatalf("cancellation priority lost: %v", err)
		}
		var staged interface{ SafeAgentFailureStage() string }
		if errors.As(err, &staged) {
			t.Fatalf("cancellation was mislabeled as %q", staged.SafeAgentFailureStage())
		}
	}
}
