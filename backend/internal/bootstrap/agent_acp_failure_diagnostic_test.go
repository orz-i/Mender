package bootstrap

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
)

// Integration proof: the real Agent Service persists a trusted ACP stage,
// never its underlying launch error or a raw failed Agent output. A disposable
// Runtime/SQLite is used; no model, production Agent, or user project is run.
func TestNativeACPFailureStagePersistsSafeTaskDiagnostic(t *testing.T) {
	const privateCause = "FAKE_BEARER_SECRET_123 /Users/private/.ssh"
	ctx := context.Background()
	runner := agentacp.Runner{
		NewCommand: func(context.Context, agentdomain.Agent) (*exec.Cmd, error) {
			return nil, errors.New(privateCause)
		},
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "disposable-agent.db"),
		AgentExecutor: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	owner := agentOwnerContext(t, runtime)
	agent, err := runtime.Agents.Create(ctx, agentapplication.CreateRequest{
		Name: "Safe failure integration fixture", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	task, err := runtime.Agents.Submit(ctx, agent.ID(), "fixture-only failure", owner)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var observed agentdomain.Task
	for time.Now().Before(deadline) {
		observed, err = runtime.Agents.Task(ctx, task.ID, owner)
		if err != nil {
			t.Fatal(err)
		}
		if observed.Status.Terminal() {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if observed.Status != agentdomain.TaskFailed ||
		observed.Diagnostic != "Agent process could not start" ||
		observed.Result != "" ||
		strings.Contains(observed.Diagnostic, privateCause) ||
		strings.Contains(observed.Diagnostic, "/Users/") {
		t.Fatalf("unsafe ACP Task terminal diagnostic: status=%s diagnostic=%q result=%q",
			observed.Status, observed.Diagnostic, observed.Result)
	}
}
