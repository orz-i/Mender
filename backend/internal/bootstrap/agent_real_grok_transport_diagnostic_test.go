package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

// This opt-in test uses the real installed Grok ACP process, but only with a
// disposable workspace, a fresh SQLite DB and scoped test-only read permission.
// It never mutates the live owner's Agent grants or records raw Agent output.
func TestOptInRealGrokPromptTransportDiagnostics(t *testing.T) {
	if os.Getenv("MENDER_REAL_GROK_TRANSPORT_DIAGNOSTIC") != "1" {
		t.Skip("live Grok model probe is opt-in only")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("native Grok ACP probe runs on the owner's macOS device only")
	}
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const fixtureContent = "mender-disposable-grok-transport-marker\n"
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte(fixtureContent), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, err := agentacp.NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Availability(agentdomain.BackendGrok); err != nil {
		t.Fatal("real Grok ACP client is unavailable")
	}
	runner := agentacp.Runner{
		NewCommand:            factory.Command,
		NewCommandInWorkspace: factory.CommandInWorkspace,
		NewCommandWithConfig:  factory.CommandWithConfig,
	}
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "isolated-grok-transport.db"),
		AgentExecutor: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.Agents.Create(ctx, agentapp.CreateRequest{
		Name: "Disposable Grok prompt transport probe", Backend: agentdomain.BackendGrok,
		WorkspaceRoot: root, Isolation: "host",
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if choices, e := r.Agents.InspectModels(ctx, agent.ID(), owner); e == nil {
		modelAdvertised := false
		for _, entry := range choices.Models {
			modelAdvertised = modelAdvertised || entry.ID == "grok-4.7"
		}
		highAdvertised := false
		if choices.ReasoningEffort != nil {
			for _, entry := range choices.ReasoningEffort.Choices {
				highAdvertised = highAdvertised || entry.ID == "high"
			}
		}
		t.Logf("grok_opt_in_model_advertised=%t, effort_high_advertised=%t", modelAdvertised, highAdvertised)
	} else {
		t.Log("grok_opt_in_session_options_unavailable")
	}
	// The test has less authority than an owner-controlled 24h Host trust:
	// only validated read requests within its temporary workspace are allowed.
	if _, err := r.Agents.SetApprovalPolicy(ctx, agent.ID(), "", agentdomain.ApprovalWorkspaceRead, owner); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = r.Agents.SetApprovalPolicy(context.Background(), agent.ID(), "", agentdomain.ApprovalManual, owner)
	}()

	submitAndObserve := func(label, prompt string, options agentapp.SubmitOptions) agentdomain.Task {
		t.Helper()
		task, err := r.Agents.SubmitWithOptions(ctx, agent.ID(), prompt, options, owner)
		if err != nil {
			t.Fatalf("%s: test Task could not be admitted: %v", label, err)
		}
		var current agentdomain.Task
		seconds := 55
		if label == "read_disposable_fixture" {
			seconds = 30 // distinguish a stalled tool request from a completed prompt
		}
		limit := time.Now().Add(time.Duration(seconds) * time.Second)
		for time.Now().Before(limit) {
			current, err = r.Agents.Task(ctx, task.ID, owner)
			if err != nil {
				t.Fatalf("%s: task state unavailable: %v", label, err)
			}
			if current.Status.Terminal() {
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
		timedOut := !current.Status.Terminal()
		// A transport diagnostic test records a bounded stalled request; it
		// does not pretend that an unresponsive Agent completed successfully.
		// Inspect only the safe ACP kind/status counters before cancellation.
		receipts, err := r.Agents.ListApprovals(ctx, task.ID, owner)
		if err != nil {
			t.Fatal(err)
		}
		var readOnce, otherOnce, pending, rejected, unknownKind int
		for _, a := range receipts {
			if a.Kind != "read" && a.Kind != "edit" && a.Kind != "execute" {
				unknownKind++
			}
			switch a.Status {
			case agentdomain.ApprovalPending:
				pending++
			case agentdomain.ApprovalRejected:
				rejected++
			case agentdomain.ApprovalAllowedOnce:
				if a.DecisionSource == "policy" {
					if a.Kind == "read" {
						readOnce++
					} else {
						otherOnce++
					}
				}
			}
		}
		t.Logf("grok_probe=%s, task_timed_out=%t, terminal=%s, safe_diagnostic=%q, policy_read_once=%d, other_policy_allows=%d, pending=%d, rejected=%d, unknown_action_kinds=%d, has_fixture_marker=%t",
			label, timedOut, current.Status, current.Diagnostic, readOnce, otherOnce, pending, rejected, unknownKind,
			strings.Contains(current.Result, "mender-disposable-grok-transport-marker"))
		if timedOut {
			_ = r.Agents.Cancel(ctx, task.ID, owner)
			// Runtime Close also stops the subprocess. Never leave a temporary
			// Task waiting for an owner click after this bounded probe ends.
			return current
		}
		if otherOnce != 0 {
			t.Fatalf("%s: read-only test grant unexpectedly authorized native writes/execute", label)
		}
		return current
	}
	// Control task has no reason to use any file/tool; the second task is the
	// minimal native read scenario that has historically been unreliable.
	if os.Getenv("MENDER_GROK_PROBE_ONLY_READ") != "1" {
		submitAndObserve("no_tools_backend_default", "Do not use any tools or file access. Reply only with GROK_NO_TOOL_CONTROL_OK.", agentapp.SubmitOptions{})
	}
	submitAndObserve("read_disposable_fixture",
		"Use your native read capability to read only marker.txt from the current workspace. "+
			"Reply with its marker text. Do not edit, execute commands, access credentials, other paths, or unrelated networks.", agentapp.SubmitOptions{})
	actual, err := os.ReadFile(filepath.Join(root, "marker.txt"))
	if err != nil || string(actual) != fixtureContent {
		t.Fatal("real Agent changed a disposable read-only marker")
	}
}
