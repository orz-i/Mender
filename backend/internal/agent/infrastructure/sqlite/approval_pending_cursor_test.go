package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestTargetedApprovalQueueCursorSkipsUnresolvedEarlyPages(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "pending-pagination.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	agent, root := scopeFixtureAgent(t, s, "Pending pagination fixture")
	if err := s.SetApprovalPolicy(ctx, agentdomain.ApprovalPolicy{
		AgentID: agent.ID(), WorkspaceRoot: root,
		Mode: agentdomain.ApprovalWorkspaceRead, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	// All 61 requests are eligible at the SQLite filter level. Their
	// filesystem previews are intentionally not checked in this query, as
	// the application may have to leave the first 50 pending for an owner.
	var all []agentdomain.ApprovalID
	for i := 0; i < 61; i++ {
		task := scopeFixtureTask(t, s, agent, "", root)
		approval := newFixtureApproval(t, agent, task, "file-pagination")
		if err := s.CreateApproval(ctx, approval); err != nil {
			t.Fatal(err)
		}
		all = append(all, approval.ID)
	}
	first, err := s.ListPendingApprovalsForWorkspace(ctx, agent.ID(), "", agentdomain.ApprovalWorkspaceRead, "", 50)
	if err != nil || len(first) != 50 {
		t.Fatalf("first bounded eligible page failed: %d %v", len(first), err)
	}
	// Importantly: do not resolve or modify any of the 50 preceding requests.
	// The cursor must still reach the later 11 approvals.
	second, err := s.ListPendingApprovalsForWorkspace(ctx, agent.ID(), "", agentdomain.ApprovalWorkspaceRead, first[len(first)-1].Approval.ID, 50)
	if err != nil || len(second) != 11 {
		t.Fatalf("unresolved first page blocked later requests: %d %v", len(second), err)
	}
	seen := map[agentdomain.ApprovalID]bool{}
	for _, item := range append(first, second...) {
		if seen[item.Approval.ID] {
			t.Fatal("an unresolved approval was replayed across cursor pages")
		}
		seen[item.Approval.ID] = true
	}
	for _, id := range all {
		if !seen[id] {
			t.Fatal("cursor skipped a still-pending eligible approval")
		}
	}
	if _, err := s.ListPendingApprovalsForWorkspace(ctx, agent.ID(), "", agentdomain.ApprovalWorkspaceRead, "not-an-approval", 50); !errors.Is(err, agentdomain.ErrInvalid) {
		t.Fatalf("invalid cursor accepted: %v", err)
	}
}
