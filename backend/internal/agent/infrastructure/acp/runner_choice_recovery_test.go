package acp

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
)

// A real child ACP v1 fixture proves we never execute a prompt before the
// exact configured choices have been advertised and confirmed. The second
// session is permitted once, only for an unadvertised choice.
func TestACPConfiguredChoiceRecoveryBeforeSinglePrompt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, mode, model, effort string
		wantStage                 string
		wantSessions              int
	}{
		{"healthy options use one session", "choice-once", "fixture-two", "high", "", 1},
		{"no owner model request requires no selection", "choice-once", "", "", "", 1},
		{"first session model missing", "choice-first-model-missing", "fixture-two", "xhigh", "", 2},
		{"first session effort missing", "choice-first-effort-missing", "fixture-two", "xhigh", "", 2},
		{"model missing twice fails closed", "choice-always-model-missing", "fixture-two", "high", stageConfigModelUnavailable, 0},
		{"effort missing twice fails closed", "choice-always-effort-missing", "fixture-two", "xhigh", stageConfigReasoningUnavailable, 0},
		{"confirmation mismatch never retries prompt", "choice-unconfirmed", "fixture-two", "high", stageConfigConfirmationFailed, 0},
		{"refresh RPC rejection cannot reuse earlier session", "choice-refresh-rejected", "fixture-two", "high", stageConfigRequestFailed, 0},
		{"invalid refreshed session ID is rejected", "choice-invalid-refreshed-session", "fixture-two", "high", stageProtocol, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			a := fixtureAgent(t)
			got, err := fixtureRunner(tc.mode).ExecuteConfigured(ctx, a, "safe synthetic fixture",
				agentapplication.RunConfig{
					WorkspaceRoot: a.Spec().WorkspaceRoot,
					Model:         tc.model, ReasoningEffort: tc.effort,
				})
			if tc.wantStage == "" {
				if err != nil || !strings.Contains(got, "fixture completed after") {
					t.Fatalf("configured ACP fixture did not complete: result=%q err=%v", got, err)
				}
				if !strings.Contains(got, "after "+strconv.Itoa(tc.wantSessions)+" ACP sessions") {
					t.Fatalf("session retry bound violated: %q", got)
				}
				return
			}
			if err == nil || got != "" {
				t.Fatalf("unsafe ACP prompt executed for %s: result=%q err=%v", tc.wantStage, got, err)
			}
			var staged interface{ SafeAgentFailureStage() string }
			if !errors.As(err, &staged) || staged.SafeAgentFailureStage() != tc.wantStage {
				t.Fatalf("wrong fail-closed stage: error=%v expected=%s", err, tc.wantStage)
			}
			if strings.Contains(err.Error(), "safe synthetic fixture") {
				t.Fatal("ACP failure included Task prompt text")
			}
		})
	}
}

func TestACPConfiguredChoiceErrorUsesOnlyFixedLocalLabels(t *testing.T) {
	cases := []struct {
		err   error
		stage string
	}{
		{errACPModelNotAdvertised, stageConfigModelUnavailable},
		{errACPReasoningNotAdvertised, stageConfigReasoningUnavailable},
		{errACPChoiceNotConfirmed, stageConfigConfirmationFailed},
		{errACPChoiceRequestFailed, stageConfigRequestFailed},
	}
	for _, tc := range cases {
		err := tagChoiceConfigurationError(errors.Join(tc.err, errors.New("FAKE_UNTRUSTED_PRIVATE_ERROR")))
		var staged interface{ SafeAgentFailureStage() string }
		if !errors.As(err, &staged) || staged.SafeAgentFailureStage() != tc.stage {
			t.Fatalf("unexpected choice stage %v", err)
		}
		if strings.Contains(err.Error(), "FAKE_UNTRUSTED_PRIVATE_ERROR") ||
			strings.Contains(err.Error(), "/Users/") {
			t.Fatal("configuration stage leaked underlying error content")
		}
	}
	for _, cancel := range []error{context.Canceled, context.DeadlineExceeded} {
		wrapped := errors.Join(errACPModelNotAdvertised, cancel)
		if err := tagChoiceConfigurationError(wrapped); !errors.Is(err, cancel) {
			t.Fatal("cancellation/deadline lost to ACP model stage")
		} else {
			var staged interface{ SafeAgentFailureStage() string }
			if errors.As(err, &staged) {
				t.Fatal("configuration was tagged despite cancellation")
			}
		}
	}
}
