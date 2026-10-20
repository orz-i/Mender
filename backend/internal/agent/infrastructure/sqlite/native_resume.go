package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// ResumePlan is internal read-only evidence. It never probes user Agent state,
// invokes a CLI, scans global histories or decides caller authorization.
func (s *Store) ResumePlan(ctx context.Context, parent agentdomain.TaskID) (agentdomain.SessionContinuation, error) {
	var out agentdomain.SessionContinuation
	if _, err := agentdomain.ParseTaskID(string(parent)); err != nil {
		return out, err
	}
	out.SourceTaskID = parent
	var backend string
	var load, resume int
	err := s.db.QueryRowContext(ctx, `SELECT a.session_ref,n.native_session_id,
           n.agent_id,n.backend,n.workspace_root,n.isolation,
           n.principal_kind,n.principal_ref,n.scope_kind,n.scope_ref,
           n.load_supported,n.resume_supported
      FROM agent_tasks t
      JOIN agent_execution_attempts a ON a.task_id=t.task_id AND a.generation=t.generation
      JOIN agent_native_sessions n ON n.session_ref=a.session_ref
      WHERE t.task_id=? AND t.status='completed' AND t.cancel_requested=0
        AND a.leader_reaped_at!=''
        AND EXISTS(SELECT 1 FROM agent_execution_events e
          WHERE e.task_id=t.task_id AND e.generation=t.generation AND e.event_kind='terminal_seen')
        AND t.agent_id=n.agent_id AND t.workspace_root=n.workspace_root
        AND t.principal_kind=n.principal_kind AND t.principal_ref=n.principal_ref
        AND t.scope_kind=n.scope_kind AND t.scope_ref=n.scope_ref
        AND NOT EXISTS(
          SELECT 1 FROM agent_task_continuations c
          JOIN agent_tasks child ON child.task_id=c.child_task_id
          LEFT JOIN agent_execution_attempts attempt ON attempt.task_id=child.task_id
          WHERE c.parent_task_id=t.task_id AND (
            child.status!='failed' OR
            EXISTS(SELECT 1 FROM agent_execution_events e
              WHERE e.task_id=child.task_id AND e.event_kind='prompt_sent') OR
            (attempt.task_id IS NOT NULL AND attempt.phase!='reserved' AND attempt.leader_reaped_at='')
          )
        )`,
		string(parent)).Scan(&out.SessionRef, &out.NativeSessionID, &out.AgentID, &backend, &out.WorkspaceRoot,
		&out.Isolation, &out.PrincipalKind, &out.PrincipalRef, &out.ScopeKind, &out.ScopeRef, &load, &resume)
	if errors.Is(err, sql.ErrNoRows) {
		return agentdomain.SessionContinuation{}, agentdomain.ErrResumeUnavailable
	}
	if err != nil {
		return agentdomain.SessionContinuation{}, err
	}
	out.Backend = agentdomain.Backend(backend)
	out.CanLoad = load != 0
	out.CanResume = resume != 0
	// Agy v1.4.2 supplies a pre-prompt, exactly checkable init identity
	// for --conversation. Historical completed Agy sessions were recorded
	// before this behavior was verified, so their persisted capability bits
	// remain untouched. Admit only valid native UUIDs from host isolation;
	// the new Runner MUST prove the live init ID matches before any prompt.
	if out.Backend == agentdomain.BackendAntigravity {
		// Even a legacy or synthetic capability bit cannot make an invalid
		// conversation identity eligible for Agy's strict pre-prompt gate.
		if out.Isolation != "host" || !agentdomain.ValidAntigravityConversationID(out.NativeSessionID) {
			return agentdomain.SessionContinuation{}, agentdomain.ErrResumeUnavailable
		}
		out.CanLoad = true
		out.CanResume = true
	}
	if !out.Valid() {
		return agentdomain.SessionContinuation{}, agentdomain.ErrResumeUnavailable
	}
	return out, nil
}

// CreateResumedTask reserves both a new immutable Task and its exact original
// Session lineage in one SQLite writer transaction. The native CLI is not
// started if a competing runtime already reserved the same parent/session.
// No TaskID/native SessionID may be a free-form executable choice.
func (s *Store) CreateResumedTask(ctx context.Context, child agentdomain.Task,
	plan agentdomain.SessionContinuation, maxActive int, profileID string) error {
	if err := child.Validate(); err != nil {
		return err
	}
	if child.Status != agentdomain.TaskQueued || child.CancelRequested ||
		!plan.Valid() || child.AgentID != plan.AgentID ||
		!child.OwnedBy(plan.PrincipalKind, plan.PrincipalRef, plan.ScopeKind, plan.ScopeRef) ||
		child.WorkspaceRoot != plan.WorkspaceRoot ||
		maxActive < 1 || maxActive > 32 {
		return agentdomain.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// The original Task's outcome, execution evidence, session identity and
	// current grants are rechecked in the same serialized writer statement.
	// Historical unknowns, completed/cancelled continuations, potentially
	// dispatched prompts or other active continuations cannot pass. A prior
	// failed child is retriable only with proved no-prompt execution evidence.
	const globalTaskLimit = 8
	const principalTaskLimit = 2
	res, err := tx.ExecContext(ctx, `INSERT INTO agent_tasks(
        task_id,agent_id,status,origin,principal_kind,principal_ref,scope_kind,scope_ref,
        generation,result_text,diagnostic,cancel_requested,created_at,updated_at,
        model,workspace_key,workspace_root,reasoning_effort,timeout_seconds)
      SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
      WHERE EXISTS(
        SELECT 1 FROM agent_tasks p
        JOIN agent_execution_attempts a ON a.task_id=p.task_id AND a.generation=p.generation
        JOIN agent_native_sessions n ON n.session_ref=a.session_ref
        WHERE p.task_id=? AND p.agent_id=? AND p.status='completed' AND p.cancel_requested=0
          AND p.principal_kind=? AND p.principal_ref=? AND p.scope_kind=? AND p.scope_ref=?
          AND p.workspace_root=? AND p.workspace_key=?
          AND n.session_ref=? AND n.native_session_id=? AND n.agent_id=? AND n.workspace_root=p.workspace_root
          AND n.principal_kind=p.principal_kind AND n.principal_ref=p.principal_ref
          AND n.scope_kind=p.scope_kind AND n.scope_ref=p.scope_ref
          AND n.isolation=(SELECT isolation FROM agents WHERE agent_id=p.agent_id)
          AND n.backend=(SELECT backend FROM agents WHERE agent_id=p.agent_id)
          AND a.leader_reaped_at!=''
          AND EXISTS(SELECT 1 FROM agent_execution_events e
            WHERE e.task_id=p.task_id AND e.generation=p.generation AND e.event_kind='terminal_seen')
          AND NOT EXISTS(
            SELECT 1 FROM agent_task_continuations c
            JOIN agent_tasks child ON child.task_id=c.child_task_id
            LEFT JOIN agent_execution_attempts attempt ON attempt.task_id=child.task_id
            WHERE c.parent_task_id=p.task_id AND (
              child.status!='failed' OR
              EXISTS(SELECT 1 FROM agent_execution_events e
                WHERE e.task_id=child.task_id AND e.event_kind='prompt_sent') OR
              (attempt.task_id IS NOT NULL AND attempt.phase!='reserved' AND attempt.leader_reaped_at='')
            )
          )
          AND NOT EXISTS(SELECT 1 FROM agent_task_continuations c
            JOIN agent_tasks x ON x.task_id=c.child_task_id
            WHERE c.session_ref=n.session_ref AND x.status IN ('queued','running'))
      )
      AND (SELECT COUNT(*) FROM agent_tasks WHERE agent_id=? AND status IN ('queued','running')) < ?
      AND (SELECT COUNT(*) FROM agent_tasks WHERE status IN ('queued','running')) < ?
      AND (SELECT COUNT(*) FROM agent_tasks WHERE principal_kind=? AND principal_ref=?
           AND status IN ('queued','running')) < ?
      AND (?='' OR EXISTS(SELECT 1 FROM agent_grants WHERE agent_id=? AND profile_id=?))
      AND (?='' OR EXISTS(SELECT 1 FROM agent_workspaces
          WHERE agent_id=? AND workspace_key=? AND workspace_root=?))
      AND (?='' OR ?='' OR EXISTS(SELECT 1 FROM agent_workspace_grants
          WHERE agent_id=? AND workspace_key=? AND profile_id=?))`,
		string(child.ID), string(child.AgentID), string(child.Status), child.Origin,
		child.PrincipalKind, child.PrincipalRef, child.ScopeKind, child.ScopeRef,
		child.Generation, child.Result, child.Diagnostic, 0,
		child.CreatedAt.UTC().Format(time.RFC3339Nano), child.UpdatedAt.UTC().Format(time.RFC3339Nano),
		child.Model, child.WorkspaceKey, child.WorkspaceRoot, child.ReasoningEffort, int(child.ExecutionTimeout()/time.Second),
		string(plan.SourceTaskID), string(plan.AgentID),
		child.PrincipalKind, child.PrincipalRef, child.ScopeKind, child.ScopeRef,
		child.WorkspaceRoot, child.WorkspaceKey,
		plan.SessionRef, plan.NativeSessionID, string(child.AgentID),
		string(child.AgentID), maxActive, globalTaskLimit,
		child.PrincipalKind, child.PrincipalRef, principalTaskLimit,
		profileID, string(child.AgentID), profileID,
		child.WorkspaceKey, string(child.AgentID), child.WorkspaceKey, child.WorkspaceRoot,
		profileID, child.WorkspaceKey, string(child.AgentID), child.WorkspaceKey, profileID)
	if err != nil {
		return fmt.Errorf("reserve Agent continuation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return agentdomain.ErrResumeUnavailable
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_task_continuations(child_task_id,parent_task_id,session_ref,created_at)
           VALUES(?,?,?,?)`, string(child.ID), string(plan.SourceTaskID), plan.SessionRef,
		time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("append native Agent continuation: %w", err)
	}
	return tx.Commit()
}

func (s *Store) TaskParent(ctx context.Context, child agentdomain.TaskID) (agentdomain.TaskID, error) {
	if _, err := agentdomain.ParseTaskID(string(child)); err != nil {
		return "", err
	}
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT parent_task_id FROM agent_task_continuations WHERE child_task_id=?`, string(child)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return agentdomain.ParseTaskID(id)
}
