package binding

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type desktopApprovalFixture struct{ started chan struct{} }

func (f desktopApprovalFixture) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, cfg agentapp.RunConfig) (string, error) {
	close(f.started)
	result, err := cfg.PermissionRequester(ctx, agentapp.PermissionRequest{
		SessionID: "wails-fixture", ToolCallID: "wails-operation", Kind: "read",
		Target: "fixture.txt", CanAllow: true,
		AllowOptionID: "once", RejectOptionID: "reject",
	})
	if err != nil {
		return "", err
	}
	if err := observeDesktopFixtureCompletion(cfg); err != nil {
		return "", err
	}
	if result.Choice != agentdomain.ApprovalAllowOnce {
		return "rejected", nil
	}
	return "approved", nil
}
func TestWailsOwnerApprovalIsTaskBoundAndNotProfileDelegation(t *testing.T) {
	ctx := context.Background()
	executor := desktopApprovalFixture{started: make(chan struct{})}
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "wails-approval.db"), AgentExecutor: executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	desktop := NewService(ctx, runtime)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("wails review fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := desktop.CreateAgent(CreateAgentInput{Name: "Wails ACP", Backend: "grok", WorkspaceRoot: root, Isolation: "host"})
	if err != nil {
		t.Fatal(err)
	}
	aID, err := agentdomain.ParseID(created.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginDesktop, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	task, err := runtime.Agents.Submit(ctx, aID, "synthetic no-file operation", caller)
	if err != nil {
		t.Fatal(err)
	}
	<-executor.started
	var approval AgentApproval
	deadline := time.After(4 * time.Second)
	for approval.ApprovalID == "" {
		items, err := desktop.ListAgentApprovals(string(task.ID))
		if err != nil {
			t.Fatal(err)
		}
		if len(items.Approvals) == 1 {
			approval = items.Approvals[0]
			break
		}
		select {
		case <-deadline:
			t.Fatal("Wails owner did not observe pending task approval")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if approval.Status != "pending" || approval.Kind != "read" || approval.Target != "fixture.txt" || !approval.CanAllow || approval.TaskID != string(task.ID) {
		t.Fatalf("wrong scoped approval: %+v", approval)
	}
	// The Wails projection must not expose raw ACP SessionID, option IDs, file
	// arguments, tokens, toolCall IDs or Principal/authorization internals.
	decided, err := desktop.DecideAgentApproval(approval.ApprovalID, "allow_once")
	if err != nil || decided.Status != "allowed_once" {
		t.Fatalf("local owner decision %+v %v", decided, err)
	}
	if _, err := desktop.DecideAgentApproval(approval.ApprovalID, "allow_once"); err == nil {
		t.Fatal("Wails repeated the same permission decision")
	}
	if _, err := desktop.DecideAgentApproval(approval.ApprovalID, "allow_always"); err == nil {
		t.Fatal("Wails accepted a permanent permission option")
	}
	end := time.After(4 * time.Second)
	for {
		got, err := desktop.GetAgentTask(string(task.ID))
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == "completed" {
			if got.Result != "approved" {
				t.Fatalf("wrong executor result: %+v", got)
			}
			break
		}
		select {
		case <-end:
			t.Fatalf("task stuck in %s", got.Status)
		case <-time.After(15 * time.Millisecond):
		}
	}
}
