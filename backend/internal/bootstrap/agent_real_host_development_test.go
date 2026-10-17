package bootstrap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

// Explicit opt-in real Agent integration: runs model inference and permits
// *only a disposable host workspace* through a test-owned, temporary trusted
// ACP approval policy. Never part of pnpm check or unattended CI.
func TestOptInRealOpenCodeHostCodeDevelopment(t *testing.T) {
	if os.Getenv("MENDER_REAL_HOST_CODE_DEVELOPMENT") != "opencode" {
		t.Skip("real Agent code development requires explicit operator opt-in")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS native OpenCode ACP acceptance only")
	}
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"go.mod":           "module mender.local/agent-dev-fixture\n\ngo 1.24\n",
		"multiply.go":      "package fixture\n\nfunc Multiply(a, b int) int { return a + b }\n",
		"multiply_test.go": "package fixture\n\nimport \"testing\"\nfunc TestMultiply(t *testing.T) { if got := Multiply(6, 7); got != 42 { t.Fatalf(\"expected 42, got %d\", got) } }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	factory, err := agentacp.NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Availability(agentdomain.BackendOpenCode); err != nil {
		t.Fatal(err)
	}
	runner := agentacp.Runner{
		NewCommand:            factory.Command,
		NewCommandInWorkspace: factory.CommandInWorkspace,
		NewCommandWithConfig:  factory.CommandWithConfig,
	}
	dbPath := filepath.Join(t.TempDir(), "disposable-host-development.db")
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: dbPath, AgentExecutor: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.Agents.Create(ctx, agentapp.CreateRequest{
		Name: "Temporary OpenCode coding acceptance", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: root, Isolation: "host", DefaultModel: os.Getenv("MENDER_REAL_HOST_CODE_MODEL"),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Agents.SetApprovalPolicy(ctx, agent.ID(), "", agentdomain.ApprovalTrusted, owner); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = r.Agents.SetApprovalPolicy(context.Background(), agent.ID(), "", agentdomain.ApprovalManual, owner)
	}()
	prompt := "You are implementing and testing code, NOT doing a read-only audit. " +
		"Only in this disposable workspace, fix Multiply(a,b) in multiply.go so that the existing unmodified multiply_test.go passes. " +
		"Use your native editing/Code Mode tools to actually change multiply.go. " +
		"Run go test ./... exactly once using your native execute/shell tool. " +
		"Do not modify go.mod or multiply_test.go, do not use network or external files. " +
		"Once the test succeeds, STOP using tools immediately and reply exactly: DEVELOPMENT_COMPLETED, go test PASS."
	task, err := r.Agents.Submit(ctx, agent.ID(), prompt, owner)
	if err != nil {
		t.Fatal(err)
	}
	var finished agentdomain.Task
	seconds := 180
	if override := os.Getenv("MENDER_REAL_HOST_CODE_SECONDS"); override != "" {
		if n, err := strconv.Atoi(override); err == nil && n >= 15 && n <= 300 {
			seconds = n
		}
	}
	limit := time.Now().Add(time.Duration(seconds) * time.Second)
	for time.Now().Before(limit) {
		finished, err = r.Agents.Task(ctx, task.ID, owner)
		if err != nil {
			t.Fatal(err)
		}
		if finished.Status.Terminal() {
			break
		}
		time.Sleep(180 * time.Millisecond)
	}
	if !finished.Status.Terminal() {
		history, _ := r.Agents.ListApprovals(ctx, task.ID, owner)
		kinds := map[string]int{}
		for _, approval := range history {
			kinds[approval.Kind+":"+string(approval.Status)]++
		}
		current, _ := os.ReadFile(filepath.Join(root, "multiply.go"))
		correct := strings.Contains(string(current), "a * b")
		goCmd := exec.CommandContext(ctx, "go", "test", "./...")
		goCmd.Dir = root
		goCmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
		_, testErr := goCmd.CombinedOutput()
		t.Logf("task_deadline_sec=%d, decision_count=%v, file_modified=%t, source_correct=%t, independent_go_test_pass=%t",
			seconds, kinds, !strings.Contains(string(current), "a + b"), correct, testErr == nil)
		_ = r.Agents.Cancel(ctx, task.ID, owner)
		t.Fatal("real coding Agent exceeded bounded disposable Task limit")
	}
	history, err := r.Agents.ListApprovals(ctx, task.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, a := range history {
		if a.Status == agentdomain.ApprovalAllowedOnce && a.DecisionSource == "policy" {
			counts[a.Kind]++
		}
	}
	t.Logf("real_backend=opencode, terminal=%s, approvals=%v, diagnostic=%q", finished.Status, counts, finished.Diagnostic)
	if finished.Status != agentdomain.TaskCompleted {
		t.Fatal("real Agent did not finish its coding Task")
	}
	source, err := os.ReadFile(filepath.Join(root, "multiply.go"))
	if err != nil || !strings.Contains(string(source), "a * b") {
		t.Fatal("real Agent did not edit the disposable Go source correctly")
	}
	goCmd := exec.CommandContext(ctx, "go", "test", "./...")
	goCmd.Dir = root
	goCmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
	output, err := goCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("independent disposable code verification failed: %v (bounded output: %s)", err, string(output)[:min(400, len(output))])
	}
	if counts["execute"]+counts["shell"] == 0 {
		t.Fatal("no actual ACP execute/shell approval evidence for the coding workflow")
	}
}
