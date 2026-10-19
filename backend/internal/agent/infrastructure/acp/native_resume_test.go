package acp

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func nativeResumeFixture(t *testing.T) (agentdomain.Agent, *agentdomain.SessionContinuation) {
	t.Helper()
	original := fixtureAgent(t)
	spec := original.Spec()
	spec.Isolation = "host"
	agent, err := agentdomain.NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	source, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	return agent, &agentdomain.SessionContinuation{
		SourceTaskID: source, SessionRef: "asess_" + strings.Repeat("a", 32),
		NativeSessionID: "fixture-1", AgentID: agent.ID(), Backend: agent.Spec().Backend,
		WorkspaceRoot: agent.Spec().WorkspaceRoot, Isolation: "host",
		PrincipalKind: "local_user", PrincipalRef: "os:501", ScopeKind: "catalog",
		CanLoad: true, CanResume: true,
	}
}

func TestACPNativeResumeUsesExactSessionAndNeverCreatesNewOne(t *testing.T) {
	for _, mode := range []string{"acp-resume", "acp-load"} {
		t.Run(mode, func(t *testing.T) {
			agent, plan := nativeResumeFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			var events []agentdomain.ExecutionEvent
			result, err := fixtureRunner(mode).ExecuteConfigured(ctx, agent, "fresh instruction",
				agentapp.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot,
					Resume:  plan,
					Observe: func(e agentdomain.ExecutionEvent) error { events = append(events, e); return nil },
				})
			if err != nil || result != "fixture completed" {
				t.Fatalf("resume result=%q err=%v", result, err)
			}
			found := false
			for _, e := range events {
				if e.Kind == agentdomain.EventSessionReady {
					if e.NativeSession != plan.NativeSessionID || e.ExistingSessionRef != plan.SessionRef {
						t.Fatalf("session identity drift: %+v", e)
					}
					found = true
				}
			}
			if !found {
				t.Fatalf("no resumed session evidence %+v", events)
			}
		})
	}
}

func TestACPNativeResumeFailClosedOnMismatchOrMissingCapability(t *testing.T) {
	for _, mode := range []string{"acp-mismatch", "acp-resume-rejected", "acp-no-capability"} {
		t.Run(mode, func(t *testing.T) {
			agent, plan := nativeResumeFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			var promptSent bool
			result, err := fixtureRunner(mode).ExecuteConfigured(ctx, agent, "fresh instruction",
				agentapp.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot,
					Resume: plan, Observe: func(e agentdomain.ExecutionEvent) error {
						if e.Kind == agentdomain.EventPromptSent {
							promptSent = true
						}
						return nil
					}})
			if result != "" || !errors.Is(err, agentdomain.ErrResumeUnavailable) || promptSent {
				t.Fatalf("unavailable Session resumed result=%q err=%v prompt=%t", result, err, promptSent)
			}
			if strings.Contains(err.Error(), "SECRET_IN_UNTRUSTED_REPLY") {
				t.Fatal("untrusted ACP RPC error leaked to stored diagnostic")
			}
		})
	}
}

func TestACPNativeResumeNeverBypassesRestrictedHostOrToolSupply(t *testing.T) {
	agent, plan := nativeResumeFixture(t)
	spec := agent.Spec()
	spec.Isolation = "restricted_host"
	restricted, err := agentdomain.NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	var started bool
	runner := fixtureRunner("acp-resume")
	factory := runner.NewCommand
	runner.NewCommand = func(ctx context.Context, a agentdomain.Agent) (*exec.Cmd, error) {
		started = true
		return factory(ctx, a)
	}
	result, err := runner.ExecuteConfigured(context.Background(), restricted, "fresh",
		agentapp.RunConfig{WorkspaceRoot: restricted.Spec().WorkspaceRoot, Resume: plan})
	if result != "" || err == nil || started {
		t.Fatalf("restricted resume launched=%t result=%s err=%v", started, result, err)
	}
}

// A partial ACP JSON-RPC write is not proof that the Agent never received
// an executable prompt. It must not make a failed continuation retryable.
type partialACPWriter struct{}

func (partialACPWriter) Write(b []byte) (int, error) {
	return len(b) / 2, errors.New("partial pipe write")
}
func TestACPNativeResumePartialPromptWriteRemainsUncertain(t *testing.T) {
	wire := &stream{stdin: partialACPWriter{}}
	result, err := wire.call(context.Background(), 3, "session/prompt", map[string]any{
		"sessionId": "fixture-1", "prompt": []any{map[string]string{"type": "text", "text": "fresh request"}},
	}, nil)
	if result != nil || !errors.Is(err, agentdomain.ErrOutcomeUncertain) {
		t.Fatalf("partial prompt write mistaken for safe-to-retry setup failure: result=%s err=%v", result, err)
	}
	if strings.Contains(err.Error(), "partial pipe write") {
		t.Fatal("untrusted transport error leaked")
	}
}
