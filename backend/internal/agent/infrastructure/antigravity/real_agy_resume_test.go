package antigravity_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	agy "github.com/orz-i/Mender/backend/internal/agent/infrastructure/antigravity"
	isolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

// Opt-in authenticated test using an isolated temporary working directory.
// Runs two short no-tool prompts in separate CLI processes and verifies the
// continuation is admitted only after a native pre-prompt identity match.
// No production task, Profile binding or Agent session is mutated.
func TestRealAntigravityHeadlessContinuationWithPrePromptIdentity(t *testing.T) {
	if os.Getenv("MENDER_REAL_AGY_CONTINUE_NO_TOOL") != "1" {
		t.Skip("set MENDER_REAL_AGY_CONTINUE_NO_TOOL=1 for isolated authenticated CLI continuation smoke")
	}
	// Use a shorter absolute bound: an unavailable native account/network
	// must terminate the spawned CLI even when no init/terminal is emitted.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	launcher, err := agentacp.NewLocalFactory(isolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := launcher.Availability(agentdomain.BackendAntigravity); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Disposable native Agy continuation", Backend: agentdomain.BackendAntigravity,
		PluginID: agentdomain.BackendAntigravity.PluginID(), ExtensionID: "main",
		WorkspaceRoot: root, Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:smoke",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := agy.Runner{NewCommand: launcher.AntigravityCommand,
		NewResumeCommand: launcher.AntigravityResumeCommand}
	var nativeID string
	first, err := runner.ExecuteConfigured(ctx, agent,
		"Do not use tools, files or network. Remember the exact marker MENDER_NATIVE_AGY_CONTINUE_73. Reply exactly READY.",
		agentapp.RunConfig{
			WorkspaceRoot: root,
			Observe: func(e agentdomain.ExecutionEvent) error {
				if e.Kind == agentdomain.EventSessionReady {
					if !e.CanLoad || !e.CanResume {
						t.Fatal("native continuation not advertised")
					}
					nativeID = e.NativeSession
				}
				return nil
			},
		})
	if err != nil || strings.TrimSpace(first) != "READY" ||
		!agentdomain.ValidAntigravityConversationID(nativeID) {
		t.Fatalf("first CLI process failed: ready=%v native_id_valid=%v err=%v",
			strings.TrimSpace(first) == "READY", agentdomain.ValidAntigravityConversationID(nativeID), err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	plan := &agentdomain.SessionContinuation{
		SourceTaskID: taskID, SessionRef: "asess_" + strings.Repeat("e", 32),
		NativeSessionID: nativeID, AgentID: agent.ID(), Backend: agentdomain.BackendAntigravity,
		WorkspaceRoot: root, Isolation: "host", PrincipalKind: "local_user",
		PrincipalRef: "os:smoke", ScopeKind: "catalog", CanLoad: true, CanResume: true,
	}
	var nativeVerified, dispatched bool
	second, err := runner.ExecuteConfigured(ctx, agent,
		"Without using any tools or files, output only the exact marker from my immediately previous turn.",
		agentapp.RunConfig{
			WorkspaceRoot: root, Resume: plan,
			Observe: func(e agentdomain.ExecutionEvent) error {
				switch e.Kind {
				case agentdomain.EventSessionReady:
					if dispatched || e.NativeSession != nativeID || e.ExistingSessionRef != plan.SessionRef {
						t.Fatal("native conversation was not verified before new prompt")
					}
					nativeVerified = true
				case agentdomain.EventPromptSent:
					if !nativeVerified {
						t.Fatal("new prompt dispatched before native identity check")
					}
					dispatched = true
				}
				return nil
			},
		})
	if err != nil || !nativeVerified || !dispatched ||
		strings.TrimSpace(second) != "MENDER_NATIVE_AGY_CONTINUE_73" {
		t.Fatalf("safe resumed conversation failed: verified=%v dispatched=%v response_match=%v err=%v",
			nativeVerified, dispatched, strings.TrimSpace(second) == "MENDER_NATIVE_AGY_CONTINUE_73", err)
	}
	// A non-existent but syntactically valid native ID often causes Agy to
	// silently create a new conversation. The runner must kill it BEFORE
	// sending any business prompt.
	invalid := *plan
	invalid.NativeSessionID = "00000000-0000-4000-8000-000000000000"
	var wrongPrompt bool
	_, err = runner.ExecuteConfigured(ctx, agent, "MUST_NOT_BE_SENT_TO_FALLBACK",
		agentapp.RunConfig{WorkspaceRoot: root, Resume: &invalid,
			Observe: func(e agentdomain.ExecutionEvent) error {
				if e.Kind == agentdomain.EventPromptSent {
					wrongPrompt = true
				}
				return nil
			}})
	if !errors.Is(err, agentdomain.ErrResumeUnavailable) || wrongPrompt {
		t.Fatalf("native CLI fallback was not stopped: err=%v prompt_sent=%v", err, wrongPrompt)
	}
	t.Log("verified two process Agy continuation and pre-prompt rejection for invalid native Conversation ID")
}
