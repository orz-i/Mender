package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// scopeFixtureAgent builds one running-eligible local host Agent on a real
// temporary canonical directory. Nothing outside the test database is touched.
func scopeFixtureAgent(t *testing.T, s *Store, name string) (agentdomain.Agent, string) {
	t.Helper()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: name, Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: root,
		Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctxScope(), a); err != nil {
		t.Fatal(err)
	}
	return a, root
}

// scopeFixtureTask runs one current-generation Task for an Agent, optionally
// under a registered workspace alias (key="" pins the Agent root).
func scopeFixtureTask(t *testing.T, s *Store, a agentdomain.Agent, key, root string) agentdomain.Task {
	t.Helper()
	ctx := ctxScope()
	if key != "" {
		if _, err := s.Workspace(ctx, a.ID(), key); errors.Is(err, agentdomain.ErrNotFound) {
			if err := s.BindWorkspace(ctx, a.ID(), key, root); err != nil {
				t.Fatal(err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	tid, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := agentdomain.Task{ID: tid, AgentID: a.ID(), Status: agentdomain.TaskQueued,
		Origin: "cli", PrincipalKind: "local_user", PrincipalRef: "os:501", ScopeKind: "catalog",
		Generation: 1, WorkspaceKey: key, WorkspaceRoot: root, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, tid, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	task.Status = agentdomain.TaskRunning
	return task
}

// insertPolicySourcedReceipts seeds legitimate allowed-once ledger rows with
// decision_source='policy' directly in the temporary test database.
func insertPolicySourcedReceipts(t *testing.T, s *Store, task agentdomain.Task, count int) {
	t.Helper()
	ctx := ctxScope()
	created := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	resolved := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	for i := 0; i < count; i++ {
		aid, err := agentdomain.NewApprovalID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_task_approvals(
		 approval_id,task_id,agent_id,generation,session_id,tool_call_id,
		 action_kind,target_path,can_allow,allow_option_id,reject_option_id,status,
		 created_at,expires_unix_ms,resolved_at,decision_source)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			aid, task.ID, task.AgentID, task.Generation, "cap-sess", fmt.Sprintf("policy-%03d", i),
			"read", "policy.txt", 1, "yes-once", "no", "allowed_once",
			created, task.CreatedAt.Add(time.Hour).UnixMilli(), resolved, "policy"); err != nil {
			t.Fatal(err)
		}
	}
}

func countPolicySourced(ctx context.Context, t *testing.T, s *Store, taskID agentdomain.TaskID) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_task_approvals
	 WHERE task_id=? AND decision_source='policy'`, taskID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ctxScope() context.Context { return context.Background() }

// The targeted workspace query must filter BEFORE the SQL LIMIT: more than 50
// older pending entries of an unrelated Agent buried the target in the global
// inbox, yet the Agent/workspace query still returns exactly the eligible rows.
func TestWorkspaceScopePendingListFiltersBeforeLimit(t *testing.T) {
	ctx := ctxScope()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "pending-scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	noiseAgent, _ := scopeFixtureAgent(t, s, "Unrelated noise Agent")
	noiseTask := scopeFixtureTask(t, s, noiseAgent, "", noiseAgent.Spec().WorkspaceRoot)
	older := time.Now().UTC()
	for i := 0; i < 60; i++ {
		aid, err := agentdomain.NewApprovalID()
		if err != nil {
			t.Fatal(err)
		}
		// Legitimate database rows: pending, unexpired, running Task, created
		// strictly older than every target request below.
		older = older.Add(-time.Second)
		if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_task_approvals(
		 approval_id,task_id,agent_id,generation,session_id,tool_call_id,
		 action_kind,target_path,can_allow,allow_option_id,reject_option_id,status,
		 created_at,expires_unix_ms,resolved_at,decision_source)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			aid, noiseTask.ID, noiseAgent.ID(), noiseTask.Generation, "noise-sess",
			fmt.Sprintf("noise-%d", i), "read", "noise.txt", 1, "yes-once", "no",
			"pending", older.Format(time.RFC3339Nano), older.Add(time.Hour).UnixMilli(), "", "owner"); err != nil {
			t.Fatal(err)
		}
	}

	target, root := scopeFixtureAgent(t, s, "Target workspace Agent")
	targetTask := scopeFixtureTask(t, s, target, "scope", root)
	policy := agentdomain.ApprovalPolicy{AgentID: target.ID(), WorkspaceKey: "scope",
		WorkspaceRoot: root, Mode: agentdomain.ApprovalWorkspaceRead, UpdatedAt: time.Now().UTC()}
	if err := s.SetApprovalPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	first := newFixtureApproval(t, target, targetTask, "target-1")
	if err := s.CreateApproval(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := newFixtureApproval(t, target, targetTask, "target-2")
	if err := s.CreateApproval(ctx, second); err != nil {
		t.Fatal(err)
	}

	// Control proof: the unchanged global owner inbox window of 50 excludes
	// the target entirely behind the 60 older unrelated rows.
	inbox, err := s.ListPendingApprovals(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range inbox {
		if row.Approval.AgentID == target.ID() {
			t.Fatal("global inbox unexpectedly surfaced the buried Agent; fixture cheated")
		}
	}
	if len(inbox) != 50 {
		t.Fatalf("expected the capped global inbox window, got %d", len(inbox))
	}

	rows, err := s.ListPendingApprovalsForWorkspace(ctx, target.ID(), "scope", agentdomain.ApprovalWorkspaceRead, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("target workspace hidden behind unrelated Agents: %d rows", len(rows))
	}
	got := map[string]bool{}
	for _, row := range rows {
		if row.Approval.AgentID != target.ID() || row.WorkspaceKey != "scope" || row.AgentName != target.Spec().Name {
			t.Fatalf("cross-Agent leak in targeted query: %+v", row)
		}
		got[string(row.Approval.ID)] = true
	}
	if !got[string(first.ID)] || !got[string(second.ID)] {
		t.Fatalf("missing target synchronization requests: %v", got)
	}

	// The unrelated Agent's own workspace view must stay empty.
	noiseRows, err := s.ListPendingApprovalsForWorkspace(ctx, noiseAgent.ID(), "", agentdomain.ApprovalWorkspaceRead, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(noiseRows) != 0 {
		t.Fatalf("unrelated Agent without a policy surfaced pending noise: %+v", noiseRows)
	}
}

// Ineligible requests of the SAME Agent (wrong kind for the owner-selected
// mode, or a parallel workspace with no policy) must not hide eligible ones.
func TestWorkspaceScopePendingListKeepsEligibleSameAgentRequests(t *testing.T) {
	ctx := ctxScope()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "pending-scope-eligible.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	target, root := scopeFixtureAgent(t, s, "Mixed eligibility Agent")
	wsTask := scopeFixtureTask(t, s, target, "mixed", root)
	policy := agentdomain.ApprovalPolicy{AgentID: target.ID(), WorkspaceKey: "mixed",
		WorkspaceRoot: root, Mode: agentdomain.ApprovalWorkspaceRead, UpdatedAt: time.Now().UTC()}
	if err := s.SetApprovalPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	readOne := newFixtureApproval(t, target, wsTask, "eligible-1")
	if err := s.CreateApproval(ctx, readOne); err != nil {
		t.Fatal(err)
	}
	// Same Task cap of 2 pending forces a parallel workspace Task for the
	// ineligible request. Unbound alias keeps it out of the policy view.
	otherRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	noiseTask := scopeFixtureTask(t, s, target, "other", otherRoot)
	ineligible := newFixtureApproval(t, target, noiseTask, "too-broad")
	ineligible.Kind = "execute"
	ineligible.Target = ""
	ineligible.CanAllow = false
	ineligible.ExpiresAt = time.Now().UTC().Add(time.Minute)
	if err := s.CreateApproval(ctx, ineligible); err != nil {
		t.Fatal(err)
	}
	secondTask := scopeFixtureTask(t, s, target, "mixed", root)
	readTwo := newFixtureApproval(t, target, secondTask, "eligible-2")
	if err := s.CreateApproval(ctx, readTwo); err != nil {
		t.Fatal(err)
	}

	rows, err := s.ListPendingApprovalsForWorkspace(ctx, target.ID(), "mixed", agentdomain.ApprovalWorkspaceRead, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("eligible requests hidden by same-Agent ineligible pending rows: %d", len(rows))
	}
	for _, row := range rows {
		if row.Approval.Kind != "read" || !row.Approval.CanAllow || row.WorkspaceKey != "mixed" {
			t.Fatalf("ineligible request passed the mode filter: %+v", row)
		}
	}

	// A plain global inbox window mixes unrelated/ineligible rows, which is
	// why the pre-LIMIT filters are required.
	flat, err := s.ListPendingApprovals(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	mixedIn := false
	for _, row := range flat {
		if row.WorkspaceKey == "other" {
			mixedIn = true
		}
	}
	if !mixedIn {
		t.Fatalf("fixture expected the ineligible row in the global window: %+v", flat)
	}

	// trusted mode on the same agent/workspace admits edit/execute metadata
	// shape while workspace_read cannot; 'other' kinds stay ineligible.
	trusted := agentdomain.ApprovalPolicy{AgentID: target.ID(), WorkspaceKey: "mixed",
		WorkspaceRoot: root, Mode: agentdomain.ApprovalTrusted,
		ExpiresAt: time.Now().UTC().Add(23 * time.Hour), UpdatedAt: time.Now().UTC().Add(time.Second)}
	if err := s.SetApprovalPolicy(ctx, trusted); err != nil {
		t.Fatal(err)
	}
	edit := newFixtureApproval(t, target, wsTask, "trusted-edit")
	edit.Kind = "edit"
	edit.CanAllow = false
	edit.Target = ""
	edit.ExpiresAt = time.Now().UTC().Add(time.Minute)
	if err := s.CreateApproval(ctx, edit); err != nil {
		t.Fatal(err)
	}
	trustedRows, err := s.ListPendingApprovalsForWorkspace(ctx, target.ID(), "mixed", agentdomain.ApprovalTrusted, 50)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, row := range trustedRows {
		kinds[row.Approval.Kind] = true
	}
	if !kinds["read"] || !kinds["edit"] || kinds["other"] || kinds["execute"] {
		t.Fatalf("trusted mode-eligible set wrong: %v", kinds)
	}

	// manual mode is never a policy-release path and stays forbidden.
	if _, err := s.ListPendingApprovalsForWorkspace(ctx, target.ID(), "mixed", agentdomain.ApprovalManual, 50); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("manual mode accepted for automatic release: %v", err)
	}
}

// The release path enforces the same strict per-Task <64 parity cap as
// RecordAutomaticApproval, inside one atomic UPDATE: exactly 64 prior
// policy-sourced decisions fail closed; 63 permit exactly one more.
func TestResolveApprovalByPolicyTaskCapParity(t *testing.T) {
	ctx := ctxScope()
	dbPath := filepath.Join(t.TempDir(), "approval-cap-parity.db")
	setupApprovalPolicyFenceFixture(t, dbPath)
	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	for _, cap := range []struct{ existing int }{{64}, {63}} {
		agent, root := scopeFixtureAgent(t, s, fmt.Sprintf("Cap Agent %d", cap.existing))
		task := scopeFixtureTask(t, s, agent, "cap", root)
		policy := agentdomain.ApprovalPolicy{AgentID: agent.ID(), WorkspaceKey: "cap",
			WorkspaceRoot: root, Mode: agentdomain.ApprovalWorkspaceRead, UpdatedAt: time.Now().UTC()}
		if err := s.SetApprovalPolicy(ctx, policy); err != nil {
			t.Fatal(err)
		}
		insertPolicySourcedReceipts(t, s, task, cap.existing)
		request := newFixtureApproval(t, agent, task, fmt.Sprintf("release-%d", cap.existing))
		if err := s.CreateApproval(ctx, request); err != nil {
			t.Fatal(err)
		}
		err := s.ResolveApprovalByPolicy(ctx, request.ID, agentdomain.ApprovalWorkspaceRead, 1)
		if cap.existing >= 64 {
			if !errors.Is(err, agentdomain.ErrConflict) {
				t.Fatalf("64 previous policy decisions permitted another release: %v", err)
			}
			still, err := s.GetApproval(ctx, request.ID)
			if err != nil || still.Status != agentdomain.ApprovalPending || still.DecisionSource != "owner" {
				t.Fatalf("failed release mutated pending evidence: %+v %v", still, err)
			}
			if got := countPolicySourced(ctx, t, s, task.ID); got != 64 {
				t.Fatalf("failed release recorded policy provenance: %d", got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("63 previous policy decisions failed closed unexpectedly: %v", err)
		}
		allowed, err := s.GetApproval(ctx, request.ID)
		if err != nil {
			t.Fatal(err)
		}
		if allowed.Status != agentdomain.ApprovalAllowedOnce || allowed.DecisionSource != "policy" ||
			allowed.ResolvedAt.IsZero() {
			t.Fatalf("release lacked immutable policy provenance: %+v", allowed)
		}
		if got := countPolicySourced(ctx, t, s, task.ID); got != 64 {
			t.Fatalf("expected the cap reached exactly: %d", got)
		}
		// The resolved receipt is terminal evidence and cannot be rewritten,
		// not by another Click nor by direct decision-source tampering.
		if err := s.ResolveApprovalByPolicy(ctx, request.ID, agentdomain.ApprovalWorkspaceRead, 1); !errors.Is(err, agentdomain.ErrConflict) {
			t.Fatalf("resolved policy decision replayed: %v", err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE agent_task_approvals SET decision_source='owner'
		 WHERE approval_id=?`, string(request.ID)); err == nil {
			t.Fatalf("policy provenance was rewritten after commit: %+v", allowed)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE agent_task_approvals SET status='pending'
		 WHERE approval_id=?`, string(request.ID)); err == nil {
			t.Fatal("terminal policy decision was reopened")
		}
	}
}
