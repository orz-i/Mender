package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

// Real installed Grok ACP option acceptance, opt-in only. A fresh disposable
// directory is used for each run. No session/prompt, edit, shell, MCP ToolSupply
// or owner production database/policy is accessed.
func TestOptInRealGrokModelChoiceRecoveryNoPrompt(t *testing.T) {
	if os.Getenv("MENDER_REAL_GROK_CHOICE_RECOVERY") != "1" {
		t.Skip("real Grok ACP model options need an explicit operator opt-in")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("real macOS Grok ACP probe")
	}
	factory, err := NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Availability(agentdomain.BackendGrok); err != nil {
		t.Fatal("Grok ACP executable unavailable")
	}
	runner := Runner{
		NewCommand:            factory.Command,
		NewCommandInWorkspace: factory.CommandInWorkspace,
		NewCommandWithConfig:  factory.CommandWithConfig,
	}
	failures := 0
	for attempt := 1; attempt <= 3; attempt++ {
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		id, err := agentdomain.NewID()
		if err != nil {
			t.Fatal(err)
		}
		agent, err := agentdomain.NewAgent(agentdomain.Spec{
			ID: id, Name: "Temporary ACP model choice inspector",
			Backend: agentdomain.BackendGrok, PluginID: agentdomain.BackendGrok.PluginID(),
			ExtensionID: "main", WorkspaceRoot: root, Isolation: "host",
			OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		secondSessionConfirmed := false
		out, runErr := runner.withSession(ctx, agent, agentapplication.RunConfig{
			WorkspaceRoot: root, Model: "grok-4.7", ReasoningEffort: "xhigh",
		}, func(wire *stream, _ string, _ json.RawMessage, _ <-chan error) (string, error) {
			// Successful visit means exact model/effort were both confirmed;
			// it deliberately sends no Agent prompt or native tool call.
			if attempt == 1 {
				// Verify the real Grok ACP server accepts another session/new
				// before any prompt, with the same workspace and no ToolSupply.
				fresh, err := wire.call(ctx, 6, "session/new", map[string]any{
					"cwd": root, "mcpServers": []any{},
				}, func(rpcMessage) error { return nil })
				if err != nil {
					return "", tagChoiceConfigurationError(err)
				}
				var created struct {
					SessionID string `json:"sessionId"`
				}
				if json.Unmarshal(fresh, &created) != nil || created.SessionID == "" {
					return "", tagFailure(stageProtocol, errors.New("invalid real ACP refreshed session"))
				}
				if err := applySessionChoices(ctx, wire, created.SessionID, fresh, "grok-4.7", "xhigh"); err != nil {
					return "", tagChoiceConfigurationError(err)
				}
				secondSessionConfirmed = true
			}
			return "configured-without-prompt", nil
		})
		cancel()
		success := runErr == nil && out == "configured-without-prompt"
		stage := "none"
		if !success {
			failures++
			stage = "unknown"
			var safe interface{ SafeAgentFailureStage() string }
			if errors.As(runErr, &safe) {
				stage = safe.SafeAgentFailureStage()
			}
		}
		t.Logf("grok_no_prompt_trial=%d configured=%t second_session_confirmed=%t safe_failure_stage=%s", attempt, success, secondSessionConfirmed, stage)
	}
	if failures != 0 {
		t.Fatalf("real Grok ACP configuration was not reliable across three bounded fresh sessions: failed=%d", failures)
	}
}
