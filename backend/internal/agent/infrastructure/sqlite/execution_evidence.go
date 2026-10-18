package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func newSessionRef() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return "asess_" + hex.EncodeToString(entropy[:]), nil
}

// BeginAttempt atomically binds an admitted running Task to one Runtime
// incarnation. The native CLI cannot be started before this write succeeds.
func (s *Store) BeginAttempt(ctx context.Context, t agentdomain.Task, instance string) error {
	if t.Validate() != nil || len(instance) != 37 || !strings.HasPrefix(instance, "arun_") {
		return agentdomain.ErrInvalid
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO agent_execution_attempts(
			task_id,generation,runtime_instance,phase,started_at,last_observed_at,event_count)
		SELECT ?,?,?, 'reserved',?,?,1
		WHERE EXISTS(SELECT 1 FROM agent_tasks WHERE task_id=? AND agent_id=?
			AND generation=? AND status='running' AND cancel_requested=0)`,
		string(t.ID), t.Generation, instance, now, now,
		string(t.ID), string(t.AgentID), t.Generation)
	if err != nil {
		return fmt.Errorf("reserve Agent execution attempt: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return agentdomain.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_execution_events(task_id,generation,event_kind,occurred_at)
		VALUES(?,?,'attempt_reserved',?)`, string(t.ID), t.Generation, now); err != nil {
		return fmt.Errorf("record Agent attempt reservation: %w", err)
	}
	return tx.Commit()
}

// RecordExecution accepts only an exact locally-owned generation, and only
// while the Task is still running. Every event is committed atomically with
// its bounded summary and optional private native-session binding.
func (s *Store) RecordExecution(ctx context.Context, t agentdomain.Task, instance string, event agentdomain.ExecutionEvent) error {
	if t.Validate() != nil || len(instance) != 37 || !strings.HasPrefix(instance, "arun_") {
		return agentdomain.ErrInvalid
	}
	switch event.Kind {
	case agentdomain.EventProcessStarted:
		if event.ProcessPID <= 0 {
			return agentdomain.ErrInvalid
		}
	case agentdomain.EventSessionReady:
		if event.NativeSession == "" || len(event.NativeSession) > 512 ||
			!utf8.ValidString(event.NativeSession) ||
			strings.ContainsAny(event.NativeSession, "\r\n\x00") {
			return agentdomain.ErrInvalid
		}
	case agentdomain.EventPromptSent, agentdomain.EventProgress, agentdomain.EventTerminalSeen, agentdomain.EventLeaderReaped, agentdomain.EventWorkerHeartbeat:
		if event.NativeSession != "" || event.ProcessPID != 0 {
			return agentdomain.ErrInvalid
		}
	default:
		return agentdomain.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Acquire SQLite's write lock and fence Task/status/attempt/Runtime in
	// the same statement, before any Session insert or derived event.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	phase := string(event.Kind)
	pid := 0
	if event.Kind == agentdomain.EventProcessStarted {
		pid = event.ProcessPID
	}
	// The phase admission matrix is a static SQL statement. Neither untrusted
	// events nor future plugin metadata may supply SQL fragments.
	const query = `UPDATE agent_execution_attempts SET
		phase=CASE WHEN ?='worker_heartbeat' OR (?='session_ready' AND phase IN ('prompt_sent','progress')) THEN phase ELSE ? END,
		last_observed_at=CASE WHEN ?='worker_heartbeat' THEN last_observed_at ELSE ? END,
		last_worker_heartbeat_at=CASE WHEN ?='worker_heartbeat' THEN ? ELSE last_worker_heartbeat_at END,
		process_pid=CASE WHEN ? > 0 THEN ? ELSE process_pid END,
		process_started_at=CASE WHEN ? > 0 THEN ? ELSE process_started_at END,
		last_progress_at=CASE WHEN ?='progress' THEN ? ELSE last_progress_at END,
		leader_reaped_at=CASE WHEN ?='process_started' THEN ''
			WHEN ?='leader_reaped' THEN ? ELSE leader_reaped_at END
		WHERE task_id=? AND generation=? AND runtime_instance=?
		AND (
		  (?='process_started' AND (phase='reserved' OR (phase='leader_reaped' AND session_ref IS NULL
		    AND NOT EXISTS(SELECT 1 FROM agent_execution_events e
		      WHERE e.task_id=agent_execution_attempts.task_id AND e.event_kind='prompt_sent'))))
		  OR (?='session_ready' AND phase IN ('process_started','prompt_sent','progress') AND session_ref IS NULL)
		  OR (?='prompt_sent' AND phase IN ('session_ready','process_started'))
		  OR (?='progress' AND phase IN ('prompt_sent','progress'))
		  OR (?='terminal_seen' AND phase IN ('prompt_sent','progress'))
		  OR (?='leader_reaped' AND phase!='leader_reaped')
		  OR ?='worker_heartbeat'
		)
		AND EXISTS(SELECT 1 FROM agent_tasks WHERE task_id=? AND generation=? AND status='running')`
	res, err := tx.ExecContext(ctx, query, phase, phase, phase, phase, now, phase, now,
		pid, pid, pid, now, phase, now, phase, phase, now,
		string(t.ID), t.Generation, instance,
		phase, phase, phase, phase, phase, phase, phase,
		string(t.ID), t.Generation)
	if err != nil {
		return fmt.Errorf("record Agent execution evidence: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return agentdomain.ErrConflict
	}
	if event.Kind == agentdomain.EventProcessStarted {
		var ordinal int
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(process_generation),0)+1 FROM agent_execution_processes WHERE task_id=?`,
			string(t.ID)).Scan(&ordinal); err != nil {
			return err
		}
		if ordinal > 2 {
			return agentdomain.ErrConflict // only one pre-prompt setup retry is permitted
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_execution_processes
			(task_id,process_generation,runtime_instance,process_pid,started_at) VALUES(?,?,?,?,?)`,
			string(t.ID), ordinal, instance, event.ProcessPID, now); err != nil {
			return err
		}
	}
	if event.Kind == agentdomain.EventLeaderReaped {
		res, err := tx.ExecContext(ctx, `UPDATE agent_execution_processes SET leader_reaped_at=?
			WHERE task_id=? AND runtime_instance=? AND leader_reaped_at=''
			AND process_generation=(SELECT MAX(process_generation) FROM agent_execution_processes WHERE task_id=?)`,
			now, string(t.ID), instance, string(t.ID))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n != 1 {
			return agentdomain.ErrConflict
		}
	}
	if event.Kind == agentdomain.EventSessionReady {
		ref, err := newSessionRef()
		if err != nil {
			return err
		}
		var backend, isolation string
		if err := tx.QueryRowContext(ctx, `SELECT backend,isolation FROM agents WHERE agent_id=?`,
			string(t.AgentID)).Scan(&backend, &isolation); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_native_sessions(
			session_ref,task_id,agent_id,backend,native_session_id,workspace_root,isolation,
			principal_kind,principal_ref,scope_kind,scope_ref,load_supported,resume_supported,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			ref, string(t.ID), string(t.AgentID), backend, event.NativeSession, t.WorkspaceRoot,
			isolation, t.PrincipalKind, t.PrincipalRef, t.ScopeKind, t.ScopeRef,
			boolToSQL(event.CanLoad), boolToSQL(event.CanResume), now)
		if err != nil {
			return fmt.Errorf("persist native AgentSession: %w", err)
		}
		if _, err = tx.ExecContext(ctx,
			`UPDATE agent_execution_attempts SET session_ref=? WHERE task_id=? AND generation=? AND runtime_instance=?`,
			ref, string(t.ID), t.Generation, instance); err != nil {
			return err
		}
	}
	if event.Kind != agentdomain.EventProgress && event.Kind != agentdomain.EventWorkerHeartbeat {
		// 32 categories maximum; progress and heartbeat are aggregated separately.
		res, err = tx.ExecContext(ctx, `UPDATE agent_execution_attempts SET event_count=event_count+1
			WHERE task_id=? AND generation=? AND runtime_instance=? AND event_count<32`,
			string(t.ID), t.Generation, instance)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return agentdomain.ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_execution_events(task_id,generation,event_kind,occurred_at)
			VALUES(?,?,?,?)`, string(t.ID), t.Generation, string(event.Kind), now)
		if err != nil {
			return fmt.Errorf("append Agent evidence event: %w", err)
		}
	}
	return tx.Commit()
}
func boolToSQL(v bool) int {
	if v {
		return 1
	}
	return 0
}

// TaskObservation is side-effect free. Missing attempts on historical rows
// are never associated by guessing native CLI state or filesystem diffs.
func (s *Store) TaskObservation(ctx context.Context, id agentdomain.TaskID) (*agentdomain.TaskObservation, error) {
	if _, err := agentdomain.ParseTaskID(string(id)); err != nil {
		return nil, err
	}
	var phase, last, heartbeat, reaped string
	var session sql.NullString
	var backend sql.NullString
	var canLoad, canResume sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT a.phase,a.last_observed_at,a.last_worker_heartbeat_at,
		a.leader_reaped_at,a.session_ref,s.backend,s.load_supported,s.resume_supported
		FROM agent_execution_attempts a LEFT JOIN agent_native_sessions s ON s.session_ref=a.session_ref
		WHERE a.task_id=?`, string(id)).Scan(&phase, &last, &heartbeat, &reaped, &session,
		&backend, &canLoad, &canResume)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339Nano, last)
	if err != nil {
		return nil, fmt.Errorf("invalid persisted Agent activity timestamp: %w", err)
	}
	recoverability := "unavailable"
	if session.Valid && session.String != "" &&
		(backend.String == "antigravity" || canLoad.Int64 != 0 || canResume.Int64 != 0) {
		recoverability = "unverified" // capability advertisement is not storage proof
	}
	obs := &agentdomain.TaskObservation{
		Phase: phase, Activity: "uncertain", LastObservedAt: at,
		SessionRecorded:       session.Valid && session.String != "",
		SessionRecoverability: recoverability, LeaderReaped: reaped != "",
	}
	if heartbeat != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, heartbeat)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid persisted Agent heartbeat: %w", parseErr)
		}
		obs.LastWorkerHeartbeatAt = &parsed
	}
	return obs, nil
}
