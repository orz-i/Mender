package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	policy "github.com/orz-i/Mender/backend/internal/policy/domain"
	app "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sand "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// RecordSandboxAdmission stores bounded admission metadata without operation payloads.
func (s *Store) RecordSandboxAdmission(ctx context.Context, record app.AdmissionAudit) error {
	if err := validateAdmission(record); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_admissions(operation_id,sandbox_id,action,principal_kind,principal_ref,origin,scope_kind,scope_reference,policy_kind,policy_reason,approval,status,started_at,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		record.OperationID, record.SandboxID.String(), string(record.Action), string(record.PrincipalKind), record.PrincipalRef, string(record.Origin), string(record.ScopeKind), record.ScopeReference, string(record.PolicyKind), record.PolicyReason, record.Approval, record.Status, record.StartedAt.UTC().Format(time.RFC3339Nano), record.FinishedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record sandbox admission: %w", err)
	}
	return nil
}

func (s *Store) ListSandboxAdmissions(ctx context.Context, id sand.ID, limit int) ([]app.AdmissionAudit, error) {
	if _, err := sand.ParseID(id.String()); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		return nil, fmt.Errorf("list sandbox admissions: limit must be 1..500")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT operation_id,sandbox_id,action,principal_kind,principal_ref,origin,scope_kind,scope_reference,policy_kind,policy_reason,approval,status,started_at,finished_at FROM sandbox_admissions WHERE sandbox_id = ? ORDER BY started_at DESC,operation_id DESC LIMIT ?`, id.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("list sandbox admissions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records := make([]app.AdmissionAudit, 0)
	for rows.Next() {
		var r app.AdmissionAudit
		var sandboxID, action, principalKind, origin, scope, decision, started, finished string
		if err := rows.Scan(&r.OperationID, &sandboxID, &action, &principalKind, &r.PrincipalRef, &origin, &scope, &r.ScopeReference, &decision, &r.PolicyReason, &r.Approval, &r.Status, &started, &finished); err != nil {
			return nil, fmt.Errorf("read sandbox admission: %w", err)
		}
		r.SandboxID = sand.ID(sandboxID)
		r.Action = governance.Action(action)
		r.PrincipalKind = governance.PrincipalKind(principalKind)
		r.Origin = governance.Origin(origin)
		r.ScopeKind = governance.ScopeKind(scope)
		r.PolicyKind = policy.DecisionKind(decision)
		if r.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
			return nil, err
		}
		if r.FinishedAt, err = time.Parse(time.RFC3339Nano, finished); err != nil {
			return nil, err
		}
		if err := validateAdmission(r); err != nil {
			return nil, fmt.Errorf("corrupt sandbox admission: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sandbox admissions rows: %w", err)
	}
	return records, nil
}
func validateAdmission(r app.AdmissionAudit) error {
	if len(r.OperationID) == 0 || len(r.OperationID) > 100 || r.OperationID != strings.TrimSpace(r.OperationID) {
		return errors.New("sandbox admission id invalid")
	}
	if _, err := sand.ParseID(r.SandboxID.String()); err != nil {
		return err
	}
	switch r.Action {
	case governance.ActionSandboxAcquire, governance.ActionSandboxProvision,
		governance.ActionSandboxStart, governance.ActionSandboxStop,
		governance.ActionSandboxDestroy, governance.ActionSandboxReconcile:
	default:
		return errors.New("sandbox admission action unsupported")
	}
	principal, err := governance.NewPrincipal(r.PrincipalKind, r.PrincipalRef)
	if err != nil {
		return err
	}
	var scope governance.Scope
	switch r.ScopeKind {
	case governance.ScopeCatalog:
		scope = governance.CatalogScope()
		if r.ScopeReference != "" {
			return errors.New("catalog admission with scope reference")
		}
	case governance.ScopeProfile:
		scope, err = governance.ProfileScope(r.ScopeReference)
		if err != nil {
			return err
		}
	default:
		return errors.New("sandbox admission scope unsupported")
	}
	if _, err := governance.NewContext(r.Origin, principal, scope); err != nil {
		return err
	}
	if r.PolicyKind != "" {
		if err := policy.ValidateDecisionKind(r.PolicyKind); err != nil {
			return err
		}
	}
	if len(r.PolicyReason) > 256 {
		return errors.New("sandbox admission policy reason exceeds bound")
	}
	switch r.Approval {
	case "not_required", "approved", "denied":
	default:
		return errors.New("sandbox approval status invalid")
	}
	switch r.Status {
	case "admitted", "denied", "failed":
	default:
		return errors.New("sandbox admission status invalid")
	}
	if r.StartedAt.IsZero() || r.FinishedAt.IsZero() || r.FinishedAt.Before(r.StartedAt) {
		return errors.New("sandbox admission timestamps invalid")
	}
	return nil
}
