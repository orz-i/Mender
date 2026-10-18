package acp

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// These fixtures spawn actual ACP JSON-RPC child processes. A configured
// choice that stays absent in two sessions of one process can be retried in
// exactly ONE other process before any session/prompt or native Tool call.
func TestACPConfiguredChoiceFreshProcessRecovery(t *testing.T) {
	cases := []struct {
		name, firstMode, secondMode, model, effort string
		wantStage                                  string
		wantProcesses                              int32
		wantSessionCount                           string
	}{
		{
			name:      "first subprocess missing model second process confirms",
			firstMode: "choice-always-model-missing", secondMode: "choice-once",
			model: "fixture-two", effort: "xhigh", wantProcesses: 2,
			wantSessionCount: "after 1 ACP sessions",
		},
		{
			name:      "first subprocess missing effort second process confirms",
			firstMode: "choice-always-effort-missing", secondMode: "choice-once",
			model: "fixture-two", effort: "xhigh", wantProcesses: 2,
			wantSessionCount: "after 1 ACP sessions",
		},
		{
			name:      "both processes missing model fail without prompt",
			firstMode: "choice-always-model-missing", secondMode: "choice-always-model-missing",
			model: "fixture-two", effort: "high", wantStage: stageConfigModelUnavailable, wantProcesses: 2,
		},
		{
			name:      "both processes missing effort fail without prompt",
			firstMode: "choice-always-effort-missing", secondMode: "choice-always-effort-missing",
			model: "fixture-two", effort: "xhigh", wantStage: stageConfigReasoningUnavailable, wantProcesses: 2,
		},
		{
			name:      "healthy first process does not restart",
			firstMode: "choice-once", secondMode: "choice-always-model-missing",
			model: "fixture-two", effort: "xhigh", wantProcesses: 1,
			wantSessionCount: "after 1 ACP sessions",
		},
		{
			name:      "same process second session recovers without a restart",
			firstMode: "choice-first-model-missing", secondMode: "choice-always-model-missing",
			model: "fixture-two", effort: "high", wantProcesses: 1,
			wantSessionCount: "after 2 ACP sessions",
		},
		{
			name:      "unconfirmed value never restarts or prompts",
			firstMode: "choice-unconfirmed", secondMode: "choice-once",
			model: "fixture-two", effort: "high", wantStage: stageConfigConfirmationFailed, wantProcesses: 1,
		},
		{
			name:      "refresh session request rejected cannot restart",
			firstMode: "choice-refresh-rejected", secondMode: "choice-once",
			model: "fixture-two", effort: "high", wantStage: stageConfigRequestFailed, wantProcesses: 1,
		},
		{
			name:      "protocol mismatch cannot restart",
			firstMode: "version2", secondMode: "choice-once",
			model: "fixture-two", effort: "high", wantStage: stageProtocol, wantProcesses: 1,
		},
		{
			name:      "no configured choices cannot trigger recovery",
			firstMode: "choice-once", secondMode: "choice-always-model-missing",
			wantProcesses: 1, wantSessionCount: "after 1 ACP sessions",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a := fixtureAgent(t)
			var launches atomic.Int32
			runner := fixtureRunner(tc.firstMode)
			first := runner.NewCommand
			second := fixtureRunner(tc.secondMode).NewCommand
			runner.NewCommand = func(ctx context.Context, a agentdomain.Agent) (*exec.Cmd, error) {
				switch launches.Add(1) {
				case 1:
					return first(ctx, a)
				case 2:
					return second(ctx, a)
				default:
					return nil, errors.New("third ACP subprocess attempted")
				}
			}
			value, err := runner.ExecuteConfigured(ctx, a, "synthetic prompt",
				agentapplication.RunConfig{
					WorkspaceRoot: a.Spec().WorkspaceRoot,
					Model:         tc.model, ReasoningEffort: tc.effort,
				})
			if count := launches.Load(); count != tc.wantProcesses {
				t.Fatalf("ACP subprocess count %d, wanted %d; result=%q, err=%v", count, tc.wantProcesses, value, err)
			}
			if tc.wantStage != "" {
				if value != "" || err == nil {
					t.Fatalf("unavailable choice unexpectedly executed prompt: result=%q error=%v", value, err)
				}
				var classified interface{ SafeAgentFailureStage() string }
				if !errors.As(err, &classified) || classified.SafeAgentFailureStage() != tc.wantStage {
					t.Fatalf("unavailable choice failed with wrong stage: %v", err)
				}
				if strings.Contains(err.Error(), "synthetic prompt") {
					t.Fatal("ACP failure leaked the test prompt")
				}
				return
			}
			if err != nil || !strings.Contains(value, tc.wantSessionCount) {
				t.Fatalf("configured choice was not confirmed before the single prompt: value=%q err=%v", value, err)
			}
		})
	}
}

// Governed ToolSupply carries a short-lived Task MCP bearer; never copy that
// capability into a second child process just to refresh Agent model options.
// The first child may create two sessions, but no new child is permitted.
func TestACPConfiguredChoiceCannotRestartGovernedToolSupplyProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a := fixtureAgent(t)
	runner := fixtureRunner("choice-always-model-missing-mcp-http")
	var launches atomic.Int32
	original := runner.NewCommandWithConfig
	runner.NewCommandWithConfig = func(ctx context.Context, agent agentdomain.Agent, cfg agentapplication.RunConfig) (*exec.Cmd, error) {
		launches.Add(1)
		return original(ctx, agent, cfg)
	}
	result, err := runner.ExecuteConfigured(ctx, a, "must never reach prompt", agentapplication.RunConfig{
		WorkspaceRoot: a.Spec().WorkspaceRoot,
		Model:         "fixture-two", ReasoningEffort: "high",
		ToolSupply: agentapplication.ToolSupply{
			Delivery:    agentapplication.ToolDeliveryMCPHTTP,
			Endpoint:    "http://127.0.0.1:12345/mcp",
			BearerToken: strings.Repeat("a", 64),
		},
	})
	var stage interface{ SafeAgentFailureStage() string }
	if result != "" || !errors.As(err, &stage) || stage.SafeAgentFailureStage() != stageConfigModelUnavailable {
		t.Fatalf("governed ACP unavailable model did not fail closed: result=%q err=%v", result, err)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("governed Task MCP bearer was used in %d child processes, want 1", got)
	}
}
