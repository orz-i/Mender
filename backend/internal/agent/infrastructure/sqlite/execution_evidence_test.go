package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func executionEvidenceFixture(t *testing.T) (*Store, agentdomain.Task, string) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "execution-evidence.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	aid, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	a, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: aid, Name: "Evidence test", Backend: agentdomain.BackendOpenCode,
		PluginID: agentdomain.BackendOpenCode.PluginID(), ExtensionID: "main",
		WorkspaceRoot: root, Isolation: "restricted_host",
		OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := agentdomain.Task{
		ID: taskID, AgentID: aid, Status: agentdomain.TaskQueued, WorkspaceRoot: root,
		Origin: "mcp", PrincipalKind: "service", PrincipalRef: "profile:prof_evidence",
		ScopeKind: "profile", ScopeRef: "prof_evidence", Generation: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, task.ID, 1, agentdomain.TaskQueued, agentdomain.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	task.Status = agentdomain.TaskRunning
	return s, task, "arun_" + strings.Repeat("f", 32)
}

func TestExecutionEvidencePersistsNativeSessionPrivatelyAndRejectsStaleWriter(t *testing.T) {
	s, task, instance := executionEvidenceFixture(t)
	ctx := context.Background()
	if got, err := s.TaskObservation(ctx, task.ID); err != nil || got != nil {
		t.Fatalf("unobserved historical Task %+v %v", got, err)
	}
	if err := s.BeginAttempt(ctx, task, instance); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{
		Kind: agentdomain.EventSessionReady, NativeSession: "too-early",
	}); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("session before process accepted: %v", err)
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 60001},
		{Kind: agentdomain.EventSessionReady, NativeSession: "private-native-session", CanLoad: true, CanResume: true},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventProgress},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := s.RecordExecution(ctx, task, instance, event); err != nil {
			t.Fatalf("event=%v err=%v", event.Kind, err)
		}
	}
	got, err := s.TaskObservation(ctx, task.ID)
	if err != nil || got == nil || !got.SessionRecorded || !got.LeaderReaped ||
		got.Phase != "leader_reaped" || got.SessionRecoverability != "unverified" ||
		got.LastObservedAt.IsZero() || got.Activity != "uncertain" {
		t.Fatalf("safe read projection %+v %v", got, err)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "private-native-session") || strings.Contains(string(payload), "60001") ||
		strings.Contains(string(payload), task.WorkspaceRoot) {
		t.Fatalf("sensitive native metadata leaked: %s", payload)
	}
	var sessionID, workspace, principal string
	var canLoad, canResume int
	if err := s.db.QueryRowContext(ctx, `SELECT native_session_id,workspace_root,principal_ref,
		load_supported,resume_supported FROM agent_native_sessions WHERE task_id=?`,
		string(task.ID)).Scan(&sessionID, &workspace, &principal, &canLoad, &canResume); err != nil {
		t.Fatal(err)
	}
	if sessionID != "private-native-session" || workspace != task.WorkspaceRoot ||
		principal != task.PrincipalRef || canLoad != 1 || canResume != 1 {
		t.Fatal("native Session binding drift")
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE agent_native_sessions SET native_session_id='hijack' WHERE task_id=?", string(task.ID)); err == nil {
		t.Fatal("native Session mutated")
	}
	if err := s.Transition(ctx, task.ID, 1, agentdomain.TaskRunning, agentdomain.TaskCompleted, "done", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{Kind: agentdomain.EventProgress}); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("late progress wrote past terminal fence: %v", err)
	}
	if err := s.RecordExecution(ctx, task, "arun_"+strings.Repeat("a", 32), agentdomain.ExecutionEvent{Kind: agentdomain.EventProcessStarted, ProcessPID: 60002}); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("wrong runtime owner gained process: %v", err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_execution_events WHERE task_id=?", string(task.ID)).Scan(&n); err != nil || n != 6 {
		t.Fatalf("event audit n=%d err=%v", n, err)
	}
}

func TestExecutionWorkerHeartbeatDoesNotClaimAgentProgress(t *testing.T) {
	s, task, instance := executionEvidenceFixture(t)
	ctx := context.Background()
	if err := s.BeginAttempt(ctx, task, instance); err != nil {
		t.Fatal(err)
	}
	before, err := s.TaskObservation(ctx, task.ID)
	if err != nil || before == nil || before.Phase != "reserved" {
		t.Fatalf("initial Task observation: %+v %v", before, err)
	}
	if err := s.RecordExecution(ctx, task, instance,
		agentdomain.ExecutionEvent{Kind: agentdomain.EventWorkerHeartbeat}); err != nil {
		t.Fatal(err)
	}
	after, err := s.TaskObservation(ctx, task.ID)
	if err != nil || after == nil || after.Phase != before.Phase ||
		!after.LastObservedAt.Equal(before.LastObservedAt) || after.LastWorkerHeartbeatAt == nil ||
		after.LastWorkerHeartbeatAt.IsZero() || after.Activity != "uncertain" {
		t.Fatalf("heartbeat incorrectly claimed Agent activity: %+v %v", after, err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_execution_events WHERE task_id=?`,
		string(task.ID)).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unbounded heartbeat audit: %d %v", count, err)
	}
	if err := s.Transition(ctx, task.ID, 1, agentdomain.TaskRunning, agentdomain.TaskUnknown, "", "lost worker"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, task, instance,
		agentdomain.ExecutionEvent{Kind: agentdomain.EventWorkerHeartbeat}); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("terminal Task accepted worker heartbeat: %v", err)
	}
}

func TestExecutionEvidencePrePromptProcessRetryAndNoThirdProcess(t *testing.T) {
	s, task, instance := executionEvidenceFixture(t)
	ctx := context.Background()
	if err := s.BeginAttempt(ctx, task, instance); err != nil {
		t.Fatal(err)
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 101},
		{Kind: agentdomain.EventLeaderReaped},
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 102},
		{Kind: agentdomain.EventSessionReady, NativeSession: "fresh-process"},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := s.RecordExecution(ctx, task, instance, event); err != nil {
			t.Fatalf("record %+v: %v", event, err)
		}
	}
	var n, reaped int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*),COUNT(NULLIF(leader_reaped_at,'')) FROM agent_execution_processes WHERE task_id=?", string(task.ID)).
		Scan(&n, &reaped); err != nil || n != 2 || reaped != 2 {
		t.Fatalf("separate subprocess evidence n=%d reaped=%d err=%v", n, reaped, err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{Kind: agentdomain.EventProcessStarted, ProcessPID: 103}); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("unexpected retry after prompt: %v", err)
	}
}

func TestExecutionEvidenceUnknownReconciliationDoesNotReplayOrRewrite(t *testing.T) {
	s, task, instance := executionEvidenceFixture(t)
	ctx := context.Background()
	if err := s.BeginAttempt(ctx, task, instance); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{Kind: agentdomain.EventProcessStarted, ProcessPID: 333}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{
		Kind: agentdomain.EventSessionReady, NativeSession: "old-native-session", CanLoad: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{Kind: agentdomain.EventPromptSent}); err != nil {
		t.Fatal(err)
	}
	// This deliberately models a lost runtime where a native process may
	// still have side effects. The original task must remain unknown.
	if _, err := s.db.ExecContext(ctx, "UPDATE agent_tasks SET updated_at=? WHERE task_id=?",
		time.Now().Add(-20*time.Minute).UTC().Format(time.RFC3339Nano), string(task.ID)); err != nil {
		t.Fatal(err)
	}
	count, err := s.ReconcileExpired(ctx, time.Now().Add(-12*time.Minute))
	if err != nil || count != 1 {
		t.Fatalf("unknown reconciliation %d %v", count, err)
	}
	after, err := s.GetTask(ctx, task.ID)
	if err != nil || after.Status != agentdomain.TaskUnknown || after.Generation != 2 {
		t.Fatalf("outcome rewritten: %+v %v", after, err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{Kind: agentdomain.EventTerminalSeen}); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("stale generation wrote terminal evidence: %v", err)
	}
	observation, err := s.TaskObservation(ctx, task.ID)
	if err != nil || observation == nil || observation.Phase != "prompt_sent" ||
		observation.Activity != "uncertain" || observation.SessionRecoverability != "unverified" {
		t.Fatalf("lost honest last evidence: %+v %v", observation, err)
	}
	if err := s.Transition(ctx, task.ID, 2, agentdomain.TaskUnknown, agentdomain.TaskCompleted, "", ""); err == nil {
		t.Fatal("unknown was rewritten as completed")
	}
}

func TestExecutionEvidenceRejectsMalformedNativeSessionAndNoPartialWrite(t *testing.T) {
	s, task, instance := executionEvidenceFixture(t)
	ctx := context.Background()
	if err := s.BeginAttempt(ctx, task, instance); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{Kind: agentdomain.EventProcessStarted, ProcessPID: 456}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", strings.Repeat("a", 513), "line\nbreak", "zero\x00byte"} {
		if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{
			Kind: agentdomain.EventSessionReady, NativeSession: bad,
		}); !errors.Is(err, agentdomain.ErrInvalid) {
			t.Fatalf("accepted malformed session reference of length %d: %v", len(bad), err)
		}
	}
	observation, err := s.TaskObservation(ctx, task.ID)
	if err != nil || observation == nil || observation.SessionRecorded || observation.Phase != "process_started" {
		t.Fatalf("partial session persisted: %+v %v", observation, err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{
		Kind: agentdomain.EventSessionReady, NativeSession: "real-identity",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordExecution(ctx, task, instance, agentdomain.ExecutionEvent{
		Kind: agentdomain.EventSessionReady, NativeSession: "overwritten",
	}); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("native session rebind accepted: %v", err)
	}
}
