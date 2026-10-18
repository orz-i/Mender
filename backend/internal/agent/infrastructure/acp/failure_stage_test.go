package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

var secretFixture = strings.Join([]string{"FAKE", "BEARER", "SECRET", "123", "/Users/private/key"}, "_")

// TestSafeACPFailureStages verifies that every fixed stage code is reported
// through SafeAgentFailureStage and that the message never leaks the injected
// secret-bearing payload.
func TestSafeACPFailureStages(t *testing.T) {
	cases := []struct {
		stage string
	}{
		{stageProcessLaunch},
		{stageInitialize},
		{stageSessionCreate},
		{stageConfiguration},
		{stagePermission},
		{stagePromptRejected},
		{stagePromptTransport},
		{stageProtocol},
		{stageResult},
	}

	for _, tc := range cases {
		t.Run(tc.stage, func(t *testing.T) {
			err := tagFailure(tc.stage, errors.New(secretFixture))
			if err == nil {
				t.Fatal("tagFailure returned nil for a non-nil error")
			}
			var staged interface{ SafeAgentFailureStage() string }
			if !errors.As(err, &staged) {
				t.Fatalf("tagFailure(%q, ...) does not implement SafeAgentFailureStage: %T", tc.stage, err)
			}
			if got := staged.SafeAgentFailureStage(); got != tc.stage {
				t.Fatalf("SafeAgentFailureStage() = %q, want exactly %q", got, tc.stage)
			}
			want := "Agent task failed at stage " + tc.stage
			if got := err.Error(); got != want {
				t.Fatalf("Error() = %q, want %q", got, want)
			}
			if strings.Contains(err.Error(), secretFixture) ||
				strings.Contains(err.Error(), "FAKE_BEARER_SECRET_123") ||
				strings.Contains(err.Error(), "/Users/private/key") {
				t.Fatalf("Error() leaks the secret-bearing payload: %q", err.Error())
			}
		})
	}
}

// TestSafeACPFailureMessagesHidePrivateMethod injects a fake private method
// name containing a path and token and verifies neither the typed Error()
// strings nor the stage wrapping exposes it, while errors.As still
// distinguishes prompt_rejected from result classification.
func TestSafeACPFailureMessagesHidePrivateMethod(t *testing.T) {
	privateMethod := "/private/acp/internal/session.prompt?token=" + secretFixture

	rejected := &rpcRejectedError{method: privateMethod}
	if got := rejected.Error(); got != "ACP request rejected by Agent" {
		t.Fatalf("rpcRejectedError.Error() = %q, want fixed phrase", got)
	}
	malformed := &malformedOutcomeError{method: privateMethod}
	if got := malformed.Error(); got != "ACP returned a malformed result" {
		t.Fatalf("malformedOutcomeError.Error() = %q, want fixed phrase", got)
	}

	for _, tc := range []struct {
		name  string
		cause error
		stage string
	}{
		{"rpc rejected", rejected, stagePromptRejected},
		{"malformed outcome", malformed, stageResult},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("wrap: %w", tc.cause)
			err := stagePromptError(wrapped)
			var staged interface{ SafeAgentFailureStage() string }
			if !errors.As(err, &staged) {
				t.Fatalf("stagePromptError(%T) not tagged: %v", tc.cause, err)
			}
			if got := staged.SafeAgentFailureStage(); got != tc.stage {
				t.Fatalf("stage = %q, want %q", got, tc.stage)
			}
			if !errors.Is(err, tc.cause) {
				t.Fatal("staged error must unwrap to the original cause")
			}
			if strings.Contains(err.Error(), privateMethod) ||
				strings.Contains(err.Error(), secretFixture) ||
				strings.Contains(err.Error(), "token") ||
				strings.Contains(err.Error(), "private") {
				t.Fatalf("Error() leaks the injected private method: %q", err.Error())
			}
		})
	}

	// The two causes must remain separately classifiable by type.
	var rej *rpcRejectedError
	if !errors.As(stagePromptError(rejected), &rej) {
		t.Fatal("errors.As must still classify rpcRejectedError")
	}
	var mal *malformedOutcomeError
	if !errors.As(stagePromptError(malformed), &mal) {
		t.Fatal("errors.As must still classify malformedOutcomeError")
	}
}

// TestSafeACPFailureCancellation verifies the fail-closed tagging rules and
// the cancellation-retention semantics of tagUnlessCancelled, plus the
// explicit classification paths of stagePromptError.
func TestSafeACPFailureCancellation(t *testing.T) {
	t.Run("unrecognized stage is ignored", func(t *testing.T) {
		inner := errors.New("arbitrary secret")
		err := tagFailure("arbitrary secret", inner)
		if err == nil {
			t.Fatal("tagFailure must not return nil")
		}
		if !errors.Is(err, inner) {
			t.Fatal("untagged error must be the original error")
		}
		var staged interface{ SafeAgentFailureStage() string }
		if errors.As(err, &staged) {
			t.Fatalf("invalid stage must not expose a stage, got %q", staged.SafeAgentFailureStage())
		}
	})

	t.Run("cancellation is retained", func(t *testing.T) {
		cases := []struct {
			name string
			sent error
		}{
			{"context canceled", context.Canceled},
			{"deadline exceeded", context.DeadlineExceeded},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				wrapped := fmt.Errorf("wrap: %w", tc.sent)
				err := tagUnlessCancelled(stageProcessLaunch, wrapped)
				if !errors.Is(err, wrapped) {
					t.Fatal("tagUnlessCancelled must preserve the cancellation error chain")
				}
				if !errors.Is(err, tc.sent) {
					t.Fatalf("errors.Is(err, %v) = false, want true", tc.sent)
				}
				var staged interface{ SafeAgentFailureStage() string }
				if errors.As(err, &staged) {
					t.Fatalf("cancellation must not be tagged, got stage %q", staged.SafeAgentFailureStage())
				}
			})
		}
	})

	t.Run("non-cancellation is tagged", func(t *testing.T) {
		err := tagUnlessCancelled(stageProtocol, errors.New(secretFixture))
		var staged interface{ SafeAgentFailureStage() string }
		if !errors.As(err, &staged) || staged.SafeAgentFailureStage() != stageProtocol {
			t.Fatalf("non-cancellation error must be tagged with %q, got %T", stageProtocol, err)
		}
	})

	t.Run("stagePromptError", func(t *testing.T) {
		cases := []struct {
			name  string
			err   error
			stage string
		}{
			{"explicit rpc rejection", &rpcRejectedError{method: "session/prompt"}, stagePromptRejected},
			{"permission denied", errClientPermissionDenied, stagePermission},
			{"arbitrary transport error", errors.New(secretFixture), stagePromptTransport},
			{"malformed outcome", &malformedOutcomeError{method: "session/prompt"}, stageResult},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				err := stagePromptError(tc.err)
				var staged interface{ SafeAgentFailureStage() string }
				if !errors.As(err, &staged) {
					t.Fatalf("stagePromptError(%T) not tagged: %v", tc.err, err)
				}
				if got := staged.SafeAgentFailureStage(); got != tc.stage {
					t.Fatalf("stage = %q, want %q", got, tc.stage)
				}
				if !errors.Is(err, tc.err) {
					t.Fatal("staged error must unwrap to the original cause")
				}
				if strings.Contains(err.Error(), secretFixture) ||
					strings.Contains(err.Error(), "/Users/private/key") {
					t.Fatalf("Error() leaks payload: %q", err.Error())
				}
			})
		}
	})
}
