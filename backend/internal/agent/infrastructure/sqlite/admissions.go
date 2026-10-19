package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// AdmissionEvent is the storage-facing alias for the canonical Agent
// admission evidence; no duplicate transport-specific domain model.
type AdmissionEvent = agentdomain.AdmissionEvent

func (s *Store) RecordAdmission(ctx context.Context, event AdmissionEvent) error {
	if _, err := agentdomain.ParseID(string(event.AgentID)); err != nil {
		return err
	}
	switch event.Action {
	case "agent.activate", "agent.task.submit", "agent.task.resume", "agent.task.cancel":
	default:
		return agentdomain.ErrInvalid
	}
	switch event.PolicyKind {
	case "allow", "deny", "require_approval", "failed":
	default:
		return agentdomain.ErrInvalid
	}
	switch event.Status {
	case "admitted", "denied", "failed":
	default:
		return agentdomain.ErrInvalid
	}
	if event.PrincipalKind == "" || event.PrincipalRef == "" || event.Origin == "" ||
		event.ScopeKind == "" || len(event.PrincipalRef) > 255 || len(event.ScopeRef) > 255 ||
		event.PolicyReason == "" || len(event.PolicyReason) > 160 ||
		strings.TrimSpace(event.PolicyReason) != event.PolicyReason {
		return agentdomain.ErrInvalid
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_admissions
		(agent_id,action,principal_kind,principal_ref,origin,scope_kind,scope_ref,policy_kind,policy_reason,status,occurred_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		string(event.AgentID), event.Action, event.PrincipalKind, event.PrincipalRef,
		event.Origin, event.ScopeKind, event.ScopeRef, event.PolicyKind,
		event.PolicyReason, event.Status, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record Agent admission: %w", err)
	}
	return nil
}
func (s *Store) ListAdmissions(ctx context.Context, id agentdomain.ID, limit int) ([]AdmissionEvent, error) {
	if _, err := agentdomain.ParseID(string(id)); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, agentdomain.ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, `SELECT agent_id,action,principal_kind,principal_ref,
		origin,scope_kind,scope_ref,policy_kind,policy_reason,status,occurred_at
		FROM agent_admissions WHERE agent_id=? ORDER BY event_id DESC LIMIT ?`, string(id), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []AdmissionEvent{}
	for rows.Next() {
		var ev AdmissionEvent
		var occurred string
		if err := rows.Scan(&ev.AgentID, &ev.Action, &ev.PrincipalKind, &ev.PrincipalRef,
			&ev.Origin, &ev.ScopeKind, &ev.ScopeRef, &ev.PolicyKind, &ev.PolicyReason, &ev.Status,
			&occurred); err != nil {
			return nil, err
		}
		if ev.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred); err != nil {
			return nil, err
		}
		result = append(result, ev)
	}
	return result, rows.Err()
}
