package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func completeResumeEvidenceFixture(t *testing.T) (*Store, agentdomain.Task, string, agentdomain.SessionContinuation) {
	t.Helper()
	s, task, instance := executionEvidenceFixture(t)
	ctx := context.Background()
	if err := s.BeginAttempt(ctx, task, instance); err != nil {
		t.Fatal(err)
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 8111},
		{Kind: agentdomain.EventSessionReady, NativeSession: "ses-resume-fixture", CanLoad: true, CanResume: true},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := s.RecordExecution(ctx, task, instance, event); err != nil {
			t.Fatal(event.Kind, err)
		}
	}
	if err := s.Transition(ctx, task.ID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "done", ""); err != nil {
		t.Fatal(err)
	}
	task.Status = agentdomain.TaskCompleted
	plan, err := s.ResumePlan(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, task, instance, plan
}

func newResumeChild(t *testing.T, parent agentdomain.Task) agentdomain.Task {
	t.Helper()
	id, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	return agentdomain.Task{
		ID: id, AgentID: parent.AgentID, Status: agentdomain.TaskQueued,
		Origin: parent.Origin, PrincipalKind: parent.PrincipalKind, PrincipalRef: parent.PrincipalRef,
		ScopeKind: parent.ScopeKind, ScopeRef: parent.ScopeRef, WorkspaceRoot: parent.WorkspaceRoot,
		WorkspaceKey: parent.WorkspaceKey, Model: parent.Model, ReasoningEffort: parent.ReasoningEffort,
		Generation: 1, CreatedAt: now, UpdatedAt: now, ContinuedFromTaskID: parent.ID,
	}
}

func TestNativeResumeExactEvidenceAndMultiTurnLineage(t *testing.T) {
	s, parent, instance, plan := completeResumeEvidenceFixture(t)
	ctx := context.Background()
	child := newResumeChild(t, parent)
	if err := s.CreateResumedTask(ctx, child, plan, 2, ""); err != nil {
		t.Fatal(err)
	}
	saved, err := s.GetTask(ctx, child.ID)
	if err != nil || saved.Status != agentdomain.TaskQueued {
		t.Fatalf("reserved child %+v %v", saved, err)
	}
	linked, err := s.TaskParent(ctx, child.ID)
	if err != nil || linked != parent.ID {
		t.Fatalf("wrong lineage %s %v", linked, err)
	}
	// Reservation must preclude duplicate or concurrent re-use even before
	// the child starts. No second Task is admitted for this Session.
	other := newResumeChild(t, parent)
	if err := s.CreateResumedTask(ctx, other, plan, 2, ""); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("duplicate continuation admitted: %v", err)
	}
	if err := s.Transition(ctx, child.ID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	instance2 := "arun_" + strings.Repeat("e", 32)
	if err := s.BeginAttempt(ctx, child, instance2); err != nil {
		t.Fatal(err)
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 9222},
		{Kind: agentdomain.EventSessionReady, NativeSession: plan.NativeSessionID, ExistingSessionRef: plan.SessionRef, CanResume: true},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := s.RecordExecution(ctx, child, instance2, event); err != nil {
			t.Fatal(event.Kind, err)
		}
	}
	if err := s.Transition(ctx, child.ID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "continued", ""); err != nil {
		t.Fatal(err)
	}
	var distinctSessions int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_native_sessions").Scan(&distinctSessions); err != nil || distinctSessions != 1 {
		t.Fatalf("continuation invented new native session: %d %v", distinctSessions, err)
	}
	grandPlan, err := s.ResumePlan(ctx, child.ID)
	if err != nil || grandPlan.SessionRef != plan.SessionRef || grandPlan.NativeSessionID != plan.NativeSessionID {
		t.Fatalf("next link loses session identity %+v %v", grandPlan, err)
	}
	grandchild := newResumeChild(t, child)
	if err := s.CreateResumedTask(ctx, grandchild, grandPlan, 2, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := s.TaskParent(ctx, grandchild.ID); err != nil || got != child.ID {
		t.Fatalf("missing latest lineage %s %v", got, err)
	}
	_ = instance
}

func TestNativeResumeRejectsWrongSourceCallerAndNativeIdentity(t *testing.T) {
	s, parent, _, plan := completeResumeEvidenceFixture(t)
	ctx := context.Background()
	wrong := newResumeChild(t, parent)
	wrong.PrincipalRef = "profile:other"
	if err := s.CreateResumedTask(ctx, wrong, plan, 2, ""); !errors.Is(err, agentdomain.ErrInvalid) {
		t.Fatalf("cross-principal continuation accepted %v", err)
	}
	forged := plan
	forged.SessionRef = "asess_" + strings.Repeat("a", 32)
	if err := s.CreateResumedTask(ctx, newResumeChild(t, parent), forged, 2, ""); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("forged SessionRef accepted %v", err)
	}
	forged = plan
	forged.NativeSessionID = "another-valid-but-unbound-native-id"
	if err := s.CreateResumedTask(ctx, newResumeChild(t, parent), forged, 2, ""); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("forged native SessionID accepted %v", err)
	}
	child := newResumeChild(t, parent)
	if err := s.CreateResumedTask(ctx, child, plan, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, child.ID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	instance := "arun_" + strings.Repeat("e", 32)
	if err := s.BeginAttempt(ctx, child, instance); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, child, instance, agentdomain.ExecutionEvent{Kind: agentdomain.EventProcessStarted, ProcessPID: 9333}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, child, instance, agentdomain.ExecutionEvent{
		Kind: agentdomain.EventSessionReady, NativeSession: "other-native",
		ExistingSessionRef: plan.SessionRef,
	}); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("native ID rebind accepted %v", err)
	}
	obs, err := s.TaskObservation(ctx, child.ID)
	if err != nil || obs == nil || obs.SessionRecorded {
		t.Fatalf("unverified session persisted %+v %v", obs, err)
	}
}

func TestNativeResumeUnknownAndUnreapedCannotBeContinued(t *testing.T) {
	s, task, instance := executionEvidenceFixture(t)
	ctx := context.Background()
	if _, err := s.ResumePlan(ctx, task.ID); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("running Task resumed %v", err)
	}
	if err := s.BeginAttempt(ctx, task, instance); err != nil {
		t.Fatal(err)
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 9333},
		{Kind: agentdomain.EventSessionReady, NativeSession: "still-running", CanLoad: true},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
	} {
		if err := s.RecordExecution(ctx, task, instance, event); err != nil {
			t.Fatal(err)
		}
	}
	// A terminal frame without reaping is no longer allowed to claim
	// completion; a Session identity cannot substitute for process evidence.
	if err := s.Transition(ctx, task.ID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "result", ""); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("unreaped Task marked completed: %v", err)
	}
	if _, err := s.ResumePlan(ctx, task.ID); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("unreaped parent resumed %v", err)
	}
	s2, task2, instance2 := executionEvidenceFixture(t)
	if err := s2.BeginAttempt(ctx, task2, instance2); err != nil {
		t.Fatal(err)
	}
	for _, e := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 9444},
		{Kind: agentdomain.EventSessionReady, NativeSession: "unknown", CanLoad: true},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := s2.RecordExecution(ctx, task2, instance2, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := s2.Transition(ctx, task2.ID, 1, agentdomain.TaskRunning, agentdomain.TaskUnknown, "", "outcome uncertain"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.ResumePlan(ctx, task2.ID); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("historical unknown resumed %v", err)
	}
}

func TestNativeResumeConcurrentReservationOnlyOneWins(t *testing.T) {
	s, parent, _, plan := completeResumeEvidenceFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			child := agentdomain.Task{
				ID:      func() agentdomain.TaskID { id, _ := agentdomain.NewTaskID(); return id }(),
				AgentID: parent.AgentID, Status: agentdomain.TaskQueued, Origin: parent.Origin,
				PrincipalKind: parent.PrincipalKind, PrincipalRef: parent.PrincipalRef,
				ScopeKind: parent.ScopeKind, ScopeRef: parent.ScopeRef, WorkspaceRoot: parent.WorkspaceRoot,
				Generation: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}
			results <- s.CreateResumedTask(ctx, child, plan, 2, "")
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
			continue
		}
		if !errors.Is(err, agentdomain.ErrResumeUnavailable) {
			t.Fatalf("unexpected concurrent failure %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("concurrent resume created %d tasks", success)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_task_continuations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed atomic ledger check count=%d err=%v", count, err)
	}
}

func TestNativeResumeForwardMigrationPreservesAdmissionAndNoHiddenGrants(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "migrate.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != latestSchemaVersion {
		t.Fatalf("missing v15 %d %v", version, err)
	}
	// New action is permitted only to an explicit governance decision.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_admissions
        (agent_id,action,principal_kind,principal_ref,origin,scope_kind,scope_ref,policy_kind,policy_reason,status,occurred_at)
        VALUES('agent_fixture','agent.task.resume','local_user','os:fixture','cli','catalog','','deny','explicit_rejection','denied',?)`,
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE agent_admissions SET status='admitted' WHERE action='agent.task.resume'"); err == nil {
		t.Fatal("resume admission evidence could be rewritten")
	}
}

func TestNativeResumeExplicitRetryOnlyAfterProvenNoPromptFailure(t *testing.T) {
	ctx := context.Background()
	s, parent, _, plan := completeResumeEvidenceFixture(t)
	failed := newResumeChild(t, parent)
	if err := s.CreateResumedTask(ctx, failed, plan, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, failed.ID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	instance := "arun_" + strings.Repeat("c", 32)
	if err := s.BeginAttempt(ctx, failed, instance); err != nil {
		t.Fatal(err)
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 10001},
		{Kind: agentdomain.EventSessionReady, NativeSession: plan.NativeSessionID, ExistingSessionRef: plan.SessionRef},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := s.RecordExecution(ctx, failed, instance, event); err != nil {
			t.Fatalf("no-prompt event=%s: %v", event.Kind, err)
		}
	}
	if err := s.Transition(ctx, failed.ID, 1, agentdomain.TaskRunning, agentdomain.TaskFailed, "", "native restore rejected"); err != nil {
		t.Fatal(err)
	}
	still, err := s.ResumePlan(ctx, parent.ID)
	if err != nil || still.SessionRef != plan.SessionRef {
		t.Fatalf("false positive duplicate after no-prompt failure %+v %v", still, err)
	}
	retry := newResumeChild(t, parent)
	if err := s.CreateResumedTask(ctx, retry, plan, 2, ""); err != nil {
		t.Fatalf("safe explicit retry blocked %v", err)
	}
	if err := s.CreateResumedTask(ctx, newResumeChild(t, parent), plan, 2, ""); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("concurrent retry admitted %v", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_task_continuations WHERE parent_task_id=?", string(parent.ID)).Scan(&count); err != nil || count != 2 {
		t.Fatalf("immutable failed/active lineage lost %d %v", count, err)
	}
}

func TestNativeResumeRejectsRetryIfFailedChildEverDispatchedPrompt(t *testing.T) {
	ctx := context.Background()
	s, parent, _, plan := completeResumeEvidenceFixture(t)
	child := newResumeChild(t, parent)
	if err := s.CreateResumedTask(ctx, child, plan, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, child.ID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	instance := "arun_" + strings.Repeat("c", 32)
	if err := s.BeginAttempt(ctx, child, instance); err != nil {
		t.Fatal(err)
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 10002},
		{Kind: agentdomain.EventSessionReady, NativeSession: plan.NativeSessionID, ExistingSessionRef: plan.SessionRef},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := s.RecordExecution(ctx, child, instance, event); err != nil {
			t.Fatal(event.Kind, err)
		}
	}
	if err := s.Transition(ctx, child.ID, 1, agentdomain.TaskRunning, agentdomain.TaskFailed, "", "unknown side effects"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResumePlan(ctx, parent.ID); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("sent-prompt failed child allowed a second replay %v", err)
	}
}
