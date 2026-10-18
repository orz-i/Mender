package acp

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestACPFailureStageRunnerReportsOnlyTrustedStages(t *testing.T) {
	const fakeSecret = "FAKE_PRIVATE_BEARER_42 /Users/private/.ssh/id_test"
	cases := []struct {
		name   string
		mode   string
		model  string
		stage  string
		launch bool
	}{
		{name: "launcher", stage: stageProcessLaunch, launch: true},
		{name: "initialize rejected", mode: "reject-initialize", stage: stageInitialize},
		{name: "new session rejected", mode: "reject-session-new", stage: stageSessionCreate},
		{name: "protocol mismatch", mode: "version2", stage: stageProtocol},
		{name: "model unavailable", mode: "models", model: "fixture-unadvertised", stage: stageConfiguration},
		{name: "permission rejected", mode: "permission", stage: stagePermission},
		{name: "prompt explicitly rejected", mode: "reject-prompt", stage: stagePromptRejected},
		{name: "prompt transport malformed", mode: "invalid-json", stage: stagePromptMalformedFrame},
		{name: "invalid completion update", mode: "foreign-session", stage: stageResult},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			agent := fixtureAgent(t)
			runner := fixtureRunner(test.mode)
			if test.launch {
				runner = Runner{NewCommand: func(context.Context, agentdomain.Agent) (*exec.Cmd, error) {
					return nil, errors.New(fakeSecret)
				}}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			result, err := runner.ExecuteConfigured(ctx, agent, "bounded private test",
				agentapplication.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot, Model: test.model})
			if err == nil || result != "" {
				t.Fatalf("failed ACP fixture unexpectedly passed: result=%q err=%v", result, err)
			}
			var staged interface{ SafeAgentFailureStage() string }
			if !errors.As(err, &staged) || staged.SafeAgentFailureStage() != test.stage {
				t.Fatalf("ACP failure stage got %v; want %q", err, test.stage)
			}
			if strings.Contains(err.Error(), fakeSecret) ||
				strings.Contains(err.Error(), "/Users/") ||
				strings.Contains(err.Error(), "Bearer ") {
				t.Fatalf("ACP failure Error leaked remote payload or local path: %q", err.Error())
			}
		})
	}
}

func TestACPFailureStageRunnerPreservesDeadlineExceeded(t *testing.T) {
	agent := fixtureAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := fixtureRunner("hang").ExecuteConfigured(ctx, agent, "timeout",
		agentapplication.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout no longer has the required lifecycle precedence: %v", err)
	}
}
