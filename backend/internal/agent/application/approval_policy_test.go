package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestOwnerConfiguredWorkspaceReadAutomaticallyAllowsWithoutWaiting(t *testing.T) {
	ctx := context.Background()
	s, ownerCtx, exec := newApprovalFixture(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	agent, err := s.Create(ctx, CreateRequest{Name: "Read grant", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: root}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, agent.ID(), goodProfileID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetApprovalPolicy(ctx, agent.ID(), "", agentdomain.ApprovalWorkspaceRead, profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile changed owner permission mode: %v", err)
	}
	p, err := s.SetApprovalPolicy(ctx, agent.ID(), "", agentdomain.ApprovalWorkspaceRead, ownerCtx)
	if err != nil || p.Mode != agentdomain.ApprovalWorkspaceRead {
		t.Fatal(p, err)
	}
	task, err := s.Submit(ctx, agent.ID(), "safe automated read", profile(t, goodProfileID))
	if err != nil {
		t.Fatal(err)
	}
	<-exec.ready
	select {
	case got := <-exec.decision:
		if got != agentdomain.ApprovalAllowOnce {
			t.Fatal(got)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("read mode did not resolve without owner")
	}
	_ = waitTask(t, s, task.ID, ownerCtx, agentdomain.TaskCompleted)
	list, err := s.ListApprovals(ctx, task.ID, ownerCtx)
	if err != nil || len(list) != 1 || list[0].Status != agentdomain.ApprovalAllowedOnce || list[0].DecisionSource != "policy" {
		t.Fatalf("unrecorded automatic approval: %+v %v", list, err)
	}
	pending, err := s.ListPendingApprovals(ctx, ownerCtx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("automatic request leaked into owner inbox: %+v %v", pending, err)
	}
}
func TestExplicitTrustedModePermitsUnreviewableKnownACPAction(t *testing.T) {
	ctx := context.Background()
	s, ownerCtx, exec := newApprovalFixture(t)
	exec.unreviewable = true
	agent, err := s.Create(ctx, CreateRequest{Name: "Trusted test", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := s.SetApprovalPolicy(ctx, agent.ID(), "", agentdomain.ApprovalTrusted, ownerCtx)
	if err != nil || policy.Mode != agentdomain.ApprovalTrusted ||
		policy.ExpiresAt.Before(time.Now().Add(23*time.Hour)) || policy.ExpiresAt.After(time.Now().Add(agentdomain.MaxTrustedDuration+time.Minute)) {
		t.Fatalf("trusted grant not bounded to 24 hours: %+v %v", policy, err)
	}
	task, err := s.Submit(ctx, agent.ID(), "explicit owner-authorized trust", ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	<-exec.ready
	select {
	case decision := <-exec.decision:
		if decision != agentdomain.ApprovalAllowOnce {
			t.Fatal(decision)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("trusted mode hung on execute request")
	}
	_ = waitTask(t, s, task.ID, ownerCtx, agentdomain.TaskCompleted)
	list, err := s.ListApprovals(ctx, task.ID, ownerCtx)
	if err != nil || len(list) != 1 || list[0].Kind != "execute" || list[0].CanAllow || list[0].DecisionSource != "policy" {
		t.Fatalf("explicit trusted decision failed: %+v %v", list, err)
	}
	if _, err := s.SetApprovalPolicy(ctx, agent.ID(), "", agentdomain.ApprovalManual, ownerCtx); err != nil {
		t.Fatal(err)
	}
	current, err := s.GetApprovalPolicy(ctx, agent.ID(), "", ownerCtx)
	if err != nil || current.Mode != agentdomain.ApprovalManual {
		t.Fatalf("trust not revoked: %+v %v", current, err)
	}
}

type sequentialReadExecutor struct {
	first     chan struct{}
	resume    chan struct{}
	decisions chan agentdomain.ApprovalChoice
}

func (f *sequentialReadExecutor) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, cfg RunConfig) (string, error) {
	for i := 0; i < 2; i++ {
		decision, err := cfg.PermissionRequester(ctx, PermissionRequest{
			SessionID: "session-revoke", ToolCallID: []string{"read-a", "read-b"}[i],
			Kind: "read", Target: []string{"one.txt", "two.txt"}[i], CanAllow: true,
			AllowOptionID: "approve-once", RejectOptionID: "reject-once",
		})
		if err != nil {
			return "", err
		}
		f.decisions <- decision.Choice
		if i == 0 {
			close(f.first)
			select {
			case <-f.resume:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	if err := observeFixtureTerminal(cfg); err != nil {
		return "", err
	}
	return "rechecked", nil
}
func TestPolicyRevocationStopsFutureReadRequestsAndOwnerInboxSeesNext(t *testing.T) {
	ctx := context.Background()
	runner := &sequentialReadExecutor{first: make(chan struct{}), resume: make(chan struct{}), decisions: make(chan agentdomain.ApprovalChoice, 2)}
	s, ownerCtx := newFixtureService(t, runner)
	root := t.TempDir()
	for _, name := range []string{"one.txt", "two.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := s.Create(ctx, CreateRequest{Name: "Revocable approval", Backend: agentdomain.BackendGrok, WorkspaceRoot: root}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, a.ID(), goodProfileID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalWorkspaceRead, ownerCtx); err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(ctx, a.ID(), "revoke after first read", profile(t, goodProfileID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.first:
	case <-time.After(3 * time.Second):
		t.Fatal("first auto read not processed")
	}
	select {
	case choice := <-runner.decisions:
		if choice != agentdomain.ApprovalAllowOnce {
			t.Fatal(choice)
		}
	default:
		t.Fatal("first read not allowed")
	}
	if _, err := s.SetApprovalPolicy(ctx, a.ID(), "", agentdomain.ApprovalManual, ownerCtx); err != nil {
		t.Fatal(err)
	}
	close(runner.resume)
	var pending []agentdomain.PendingApproval
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pending, err = s.ListPendingApprovals(ctx, ownerCtx)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pending) != 1 || pending[0].Approval.TaskID != task.ID || pending[0].WorkspaceKey != "" || pending[0].AgentName != "Revocable approval" || pending[0].Approval.Target != "two.txt" {
		t.Fatalf("pending global inbox did not surface second read: %+v", pending)
	}
	if _, err := s.ListPendingApprovals(ctx, profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile can browse owner inbox: %v", err)
	}
	if _, err := s.DecideApproval(ctx, pending[0].Approval.ID, agentdomain.ApprovalReject, ownerCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case choice := <-runner.decisions:
		if choice != agentdomain.ApprovalReject {
			t.Fatal(choice)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second decision was not delivered")
	}
	_ = waitTask(t, s, task.ID, ownerCtx, agentdomain.TaskCompleted)
	ledger, err := s.ListApprovals(ctx, task.ID, ownerCtx)
	if err != nil || len(ledger) != 2 {
		t.Fatalf("lost per-request ledger: %+v %v", ledger, err)
	}
	var manual, automatic int
	for _, entry := range ledger {
		if entry.DecisionSource == "policy" && entry.Status == agentdomain.ApprovalAllowedOnce {
			automatic++
		}
		if entry.DecisionSource == "owner" && entry.Status == agentdomain.ApprovalRejected {
			manual++
		}
	}
	if manual != 1 || automatic != 1 {
		t.Fatalf("unexpected decision sources: %+v", ledger)
	}
}
