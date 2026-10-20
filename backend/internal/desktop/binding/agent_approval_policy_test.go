package binding

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type desktopNativeTrustFixture struct {
	started  chan struct{}
	decision chan agentdomain.ApprovalChoice
}

func (f desktopNativeTrustFixture) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, cfg agentapp.RunConfig) (string, error) {
	close(f.started)
	result, err := cfg.PermissionRequester(ctx, agentapp.PermissionRequest{
		SessionID: "wails-trusted-session", ToolCallID: "wails-native-edit",
		Kind: "edit", CanAllow: false, Target: "",
		AllowOptionID: "once", RejectOptionID: "no",
	})
	if err != nil {
		return "", err
	}
	f.decision <- result.Choice
	if err := observeDesktopFixtureCompletion(cfg); err != nil {
		return "", err
	}
	return "Wails trustee approval observed", nil
}
func TestWailsOwnerCentralInboxAndScopedPendingRelease(t *testing.T) {
	ctx := context.Background()
	runner := desktopNativeTrustFixture{started: make(chan struct{}), decision: make(chan agentdomain.ApprovalChoice, 1)}
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "owner-v12.db"), AgentExecutor: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	ownerCtx, err := gov.NewContext(gov.OriginDesktop, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	desktop := NewService(ctx, runtime)
	a, err := desktop.CreateAgent(CreateAgentInput{Name: "Owner pending fixture", Backend: "opencode",
		WorkspaceRoot: t.TempDir(), Isolation: "host"})
	if err != nil {
		t.Fatal(err)
	}
	existing, err := desktop.ListAgentApprovalPolicies(a.AgentID)
	if err != nil || len(existing.Policies) != 0 {
		t.Fatalf("fresh Host gained approval mode: %+v %v", existing, err)
	}
	defaultMode, err := desktop.GetAgentApprovalPolicy(a.AgentID, "")
	if err != nil || defaultMode.Mode != "manual" {
		t.Fatalf("default owner mode mismatch: %+v %v", defaultMode, err)
	}
	agentID, err := agentdomain.ParseID(a.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := runtime.Agents.Submit(ctx, agentID, "fixture-native-edit-request", ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	<-runner.started
	var pending AgentPendingApproval
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		items, e := desktop.ListPendingAgentApprovals()
		if e != nil {
			t.Fatal(e)
		}
		if len(items.Approvals) > 0 {
			pending = items.Approvals[0]
			break
		}
		time.Sleep(12 * time.Millisecond)
	}
	if pending.Approval.ApprovalID == "" || pending.AgentName != "Owner pending fixture" ||
		pending.WorkspaceKey != "" || pending.Approval.Kind != "edit" || pending.Approval.CanAllow ||
		pending.Approval.TaskID != string(task.ID) {
		t.Fatalf("incorrect central inbox content %+v", pending)
	}
	// Scope is human readable, but no raw ACP option IDs or prompt are exposed.
	if _, err := desktop.DecideAgentApproval(pending.Approval.ApprovalID, "allow_once"); err == nil {
		t.Fatal("unreviewable native edit was manually approved")
	}
	policy, err := desktop.SetAgentApprovalPolicy(a.AgentID, "", "trusted")
	if err != nil || policy.Mode != "trusted" || policy.ExpiresAt == "" {
		t.Fatalf("trusted mode not set %+v %v", policy, err)
	}
	receipt, err := desktop.ReleasePendingAgentApprovals(a.AgentID, "")
	if err != nil || receipt.Released != 1 {
		t.Fatalf("owner could not release pending request %+v %v", receipt, err)
	}
	select {
	case choice := <-runner.decision:
		if choice != agentdomain.ApprovalAllowOnce {
			t.Fatal(choice)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("trusted owner release didn't reach runner")
	}
	var after AgentTask
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		after, err = desktop.GetAgentTask(string(task.ID))
		if err != nil {
			t.Fatal(err)
		}
		if after.Status == "completed" {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if after.Status != "completed" {
		t.Fatalf("task did not complete: %+v", after)
	}
	list, err := desktop.ListAgentApprovals(string(task.ID))
	if err != nil || len(list.Approvals) != 1 || list.Approvals[0].DecisionSource != "policy" {
		t.Fatalf("automatic policy evidence not visible: %+v %v", list, err)
	}
	cleared, err := desktop.SetAgentApprovalPolicy(a.AgentID, "", "manual")
	if err != nil || cleared.Mode != "manual" {
		t.Fatalf("owner could not revoke trust: %+v %v", cleared, err)
	}
	remaining, err := desktop.ListAgentApprovalPolicies(a.AgentID)
	if err != nil || len(remaining.Policies) != 0 {
		t.Fatalf("owner revoked but persistence retains trust: %+v %v", remaining, err)
	}
}
