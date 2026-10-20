package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// Agy's v1.4.2 Headless --conversation capability was discovered after the
// first sessions were persisted. Do not modify historical capability bits:
// select only proven-complete Host Tasks with a canonical native UUID, and
// require the runner to prove the actual live identity before dispatch.
func TestHistoricalAntigravitySessionCanBeVerifiedWithoutRewritingStoredEvidence(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "agy-legacy-session.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Historical host Agy", Backend: agentdomain.BackendAntigravity,
		PluginID: agentdomain.BackendAntigravity.PluginID(), ExtensionID: "main",
		WorkspaceRoot: t.TempDir(), Isolation: "host",
		OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	task := agentdomain.Task{
		ID: taskID, AgentID: id, Status: agentdomain.TaskQueued,
		Origin: "mcp", PrincipalKind: "service", PrincipalRef: "profile:existing",
		ScopeKind: "profile", ScopeRef: "existing",
		WorkspaceRoot: agent.Spec().WorkspaceRoot,
		Generation:    1, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, task.ID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	task.Status = agentdomain.TaskRunning
	owner := "arun_" + strings.Repeat("f", 32)
	if err := s.BeginAttempt(ctx, task, owner); err != nil {
		t.Fatal(err)
	}
	const nativeID = "dfe3c2a6-211b-48dc-bfa8-a5d2ccf51b85"
	for _, e := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 8457},
		{Kind: agentdomain.EventSessionReady, NativeSession: nativeID}, // old metadata says 0 / 0
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := s.RecordExecution(ctx, task, owner, e); err != nil {
			t.Fatal(e.Kind, err)
		}
	}
	if err := s.Transition(ctx, task.ID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "old done", ""); err != nil {
		t.Fatal(err)
	}
	plan, err := s.ResumePlan(ctx, task.ID)
	if err != nil || plan.NativeSessionID != nativeID || !plan.Valid() || !plan.CanLoad || !plan.CanResume {
		t.Fatalf("legacy proven-complete Agy session cannot enter pre-prompt identity check: %+v err=%v", plan, err)
	}
	var load, resume int
	if err := s.db.QueryRowContext(ctx,
		"SELECT load_supported,resume_supported FROM agent_native_sessions WHERE task_id=?",
		string(task.ID)).Scan(&load, &resume); err != nil || load != 0 || resume != 0 {
		t.Fatalf("historic capability was rewritten: %d %d %v", load, resume, err)
	}
	obs, err := s.TaskObservation(ctx, task.ID)
	if err != nil || obs == nil || obs.SessionRecoverability != "unverified" {
		t.Fatalf("Agy native identity must be checked live: %+v err=%v", obs, err)
	}
	child := newResumeChild(t, task)
	if err := s.CreateResumedTask(ctx, child, plan, 2, ""); err != nil {
		t.Fatalf("serialized, caller-bound continuation reservation failed: %v", err)
	}
	other := newResumeChild(t, task)
	if err := s.CreateResumedTask(ctx, other, plan, 2, ""); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("duplicate native Session continuation admitted: %v", err)
	}
	previous, err := s.GetTask(ctx, task.ID)
	if err != nil || previous.Status != agentdomain.TaskCompleted ||
		previous.Result != "old done" || previous.Generation != 1 {
		t.Fatalf("old Task was rewritten: %+v %v", previous, err)
	}
}
