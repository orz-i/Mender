package antigravity

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func agyFixture(t *testing.T) agentdomain.Agent {
	t.Helper()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Antigravity test", Backend: agentdomain.BackendAntigravity,
		PluginID: agentdomain.BackendAntigravity.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:test", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func scriptFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake-agy.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// #nosec G302 -- private fixture script must be executable for the subprocess test.
	if err := os.Chmod(p, 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}
func runnerFixture(path string, calls *int) Runner {
	return Runner{NewCommand: func(ctx context.Context, _ agentdomain.Agent, _, _, _ string, inspect, yolo bool) (*exec.Cmd, error) {
		*calls++
		if inspect || yolo {
			return nil, errors.New("unexpected test flags")
		}
		return exec.CommandContext(ctx, path), nil
	}}
}
func TestNativeHeadlessAcceptsExactlyOneSuccessfulResultAndStdinPrompt(t *testing.T) {
	script := scriptFixture(t, `IFS= read -r prompt || exit 12
case "$prompt" in *"ONLY_FROM_STDIN"*) ;; *) exit 13;; esac
printf '%s\n' '{"event":"init","init":{"permission_mode":"request-review"}}' '{"event":"step_update","step_update":{"step_type":"tool","tool_info":{"path":"SENSITIVE"}}}' '{"event":"result","result":{"status":"SUCCESS","response":"completed"}}'`)
	calls := 0
	r := runnerFixture(script, &calls)
	a := agyFixture(t)
	out, err := r.ExecuteConfigured(context.Background(), a, "ONLY_FROM_STDIN", agentapp.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot})
	if err != nil || out != "completed" || calls != 1 {
		t.Fatalf("result=%q err=%v calls=%d", out, err, calls)
	}
}
func TestNativeHeadlessRejectsUnsafeOrIncompleteFrames(t *testing.T) {
	goodInit := "{\"event\":\"init\"}\n"
	for _, tc := range []struct{ name, stream, stage string }{
		{"missing_init", `{"event":"result","result":{"status":"SUCCESS","response":"success"}}`, "headless_protocol"},
		{"incomplete", goodInit + `{"event":"step_update"}`, "headless_protocol"},
		{"waiting", goodInit + `{"event":"result","result":{"status":"WAITING","error":"secret"}}`, "headless_permission"},
		{"failed", goodInit + `{"event":"result","result":{"status":"ERROR","error":"secret"}}`, "headless_rejected"},
		{"cancelled", goodInit + `{"event":"result","result":{"status":"CANCELED"}}`, "headless_rejected"},
		{"duplicate", goodInit + `{"event":"result","result":{"status":"SUCCESS","response":"ok"}}` + "\n" + `{"event":"result","result":{"status":"SUCCESS","response":"ok"}}`, "headless_protocol"},
		{"unknown", goodInit + `{"event":"magic"}`, "headless_protocol"},
		{"malformed", goodInit + `{"event":`, "headless_protocol"},
		{"oversized", goodInit + `{"event":"result","result":{"status":"SUCCESS","response":"` + strings.Repeat("x", agentdomain.MaxResultBytes+1) + `"}}`, "result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseStream(strings.NewReader(tc.stream + "\n"))
			var staged interface{ SafeAgentFailureStage() string }
			if !errors.As(err, &staged) || staged.SafeAgentFailureStage() != tc.stage {
				t.Fatalf("stage=%v want=%s", err, tc.stage)
			}
		})
	}
}
func TestNativeHeadlessRejectsToolSupplyBeforeStartingProcess(t *testing.T) {
	called := 0
	r := Runner{NewCommand: func(context.Context, agentdomain.Agent, string, string, string, bool, bool) (*exec.Cmd, error) {
		called++
		return nil, nil
	}}
	a := agyFixture(t)
	_, err := r.ExecuteConfigured(context.Background(), a, "do a task", agentapp.RunConfig{
		WorkspaceRoot: a.Spec().WorkspaceRoot, ToolSupply: agentapp.ToolSupply{
			Delivery: agentapp.ToolDeliveryMCPHTTP, Endpoint: "http://127.0.0.1:7777/mcp", BearerToken: strings.Repeat("a", 64),
		},
	})
	var stage interface{ SafeAgentFailureStage() string }
	if called != 0 || !errors.As(err, &stage) || stage.SafeAgentFailureStage() != "headless_tool_supply" {
		t.Fatalf("called=%d err=%v", called, err)
	}
}
func TestNativeHeadlessCancelStopsOwnedProcess(t *testing.T) {
	script := scriptFixture(t, "IFS= read -r line || exit 12\nexec sleep 20")
	a := agyFixture(t)
	calls := 0
	r := runnerFixture(script, &calls)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := r.ExecuteConfigured(ctx, a, "a short prompt", agentapp.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 4*time.Second {
		t.Fatalf("cancel=%v duration=%s", err, time.Since(start))
	}
}
func TestNativeHeadlessModelInspectionBoundedAndValidated(t *testing.T) {
	choice, err := parseModels("gemini-3.8-flash-high\tGemini High\nclaude-sonnet-4-6\tClaude Sonnet\n")
	if err != nil || len(choice.Models) != 2 || choice.Models[0].ID != "gemini-3.8-flash-high" {
		t.Fatalf("choices=%+v err=%v", choice, err)
	}
	for _, bad := range []string{"", "model without tab", "same\tName\nsame\tDuplicate", "one\t\n", "one\tName\nmalformed"} {
		if _, err := parseModels(bad); err == nil {
			t.Errorf("invalid models accepted: %q", bad)
		}
	}
}

func TestNativeHeadlessResumeFailsBeforeProcessOrPromptWithoutVerifiableHandshake(t *testing.T) {
	agent := agyFixture(t)
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	plan := &agentdomain.SessionContinuation{
		SourceTaskID: taskID, SessionRef: "asess_" + strings.Repeat("a", 32),
		NativeSessionID: "055a398f-db14-4c5f-abbb-1bf03f8120a7",
		AgentID:         agent.ID(), Backend: agentdomain.BackendAntigravity,
		WorkspaceRoot: agent.Spec().WorkspaceRoot, Isolation: "host",
		PrincipalKind: "local_user", PrincipalRef: "os:test", ScopeKind: "catalog",
	}
	starts := 0
	runner := Runner{NewCommand: func(context.Context, agentdomain.Agent, string, string, string, bool, bool) (*exec.Cmd, error) {
		starts++
		return nil, errors.New("should never launch for unverified native resume")
	}}
	result, err := runner.ExecuteConfigured(context.Background(), agent, "must not run",
		agentapp.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot, Resume: plan})
	if result != "" || !errors.Is(err, agentdomain.ErrResumeUnavailable) || starts != 0 {
		t.Fatalf("unverifiable Antigravity continuation launched: result=%q err=%v starts=%d", result, err, starts)
	}
}
