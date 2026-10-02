package sqlite

import (
	"context"
	"fmt"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func (s *Store) RecordProviderActivation(
	ctx context.Context,
	record auditdomain.ProviderActivationRecord,
) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO provider_activation_audit(
			activation_id,
			provider_id,
			action,
			principal_kind,
			principal_ref,
			origin,
			scope_kind,
			scope_reference,
			started_at,
			finished_at,
			policy_kind,
			policy_reason,
			approval,
			status,
			error_class,
			error_summary
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ActivationID(),
		record.ProviderID().String(),
		string(governancedomain.ActionProviderActivate),
		record.PrincipalKind(),
		record.PrincipalRef(),
		string(record.Origin()),
		string(record.ScopeKind()),
		record.ScopeReference(),
		formatTime(record.StartedAt()),
		formatTime(record.FinishedAt()),
		string(record.PolicyKind()),
		record.PolicyReason(),
		string(record.Approval()),
		string(record.Status()),
		string(record.ErrorClass()),
		record.ErrorSummary(),
	)
	if err != nil {
		return fmt.Errorf("record provider activation audit: %w", err)
	}
	return nil
}

func (s *Store) ListProviderActivations(
	ctx context.Context,
	limit int,
) ([]auditdomain.ProviderActivationRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			activation_id,
			provider_id,
			action,
			principal_kind,
			principal_ref,
			origin,
			scope_kind,
			scope_reference,
			started_at,
			finished_at,
			policy_kind,
			policy_reason,
			approval,
			status,
			error_class,
			error_summary
		FROM provider_activation_audit
		ORDER BY finished_at DESC, activation_id DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list provider activation audit: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := make([]auditdomain.ProviderActivationRecord, 0, limit)
	for rows.Next() {
		var (
			activationID   string
			rawProviderID  string
			action         string
			principalKind  string
			principalRef   string
			origin         string
			scopeKind      string
			scopeReference string
			rawStarted     string
			rawFinished    string
			policyKind     string
			policyReason   string
			approval       string
			status         string
			errorClass     string
			errorSummary   string
		)
		if err := rows.Scan(
			&activationID,
			&rawProviderID,
			&action,
			&principalKind,
			&principalRef,
			&origin,
			&scopeKind,
			&scopeReference,
			&rawStarted,
			&rawFinished,
			&policyKind,
			&policyReason,
			&approval,
			&status,
			&errorClass,
			&errorSummary,
		); err != nil {
			return nil, fmt.Errorf("scan provider activation audit: %w", err)
		}
		if action != string(governancedomain.ActionProviderActivate) {
			return nil, fmt.Errorf("provider activation audit action %q is invalid", action)
		}
		providerID, err := parseActivationProviderID(rawProviderID)
		if err != nil {
			return nil, err
		}
		startedAt, err := parseAuditTime(rawStarted)
		if err != nil {
			return nil, err
		}
		finishedAt, err := parseAuditTime(rawFinished)
		if err != nil {
			return nil, err
		}
		principal, err := governancedomain.NewPrincipal(
			governancedomain.PrincipalKind(principalKind),
			principalRef,
		)
		if err != nil {
			return nil, fmt.Errorf("parse provider activation principal: %w", err)
		}
		var scope governancedomain.Scope
		switch governancedomain.ScopeKind(scopeKind) {
		case governancedomain.ScopeCatalog:
			scope = governancedomain.CatalogScope()
		case governancedomain.ScopeProfile:
			scope, err = governancedomain.ProfileScope(scopeReference)
			if err != nil {
				return nil, fmt.Errorf("parse provider activation scope: %w", err)
			}
		default:
			return nil, fmt.Errorf("provider activation scope kind %q is invalid", scopeKind)
		}
		gctx, err := governancedomain.NewContext(
			governancedomain.Origin(origin),
			principal,
			scope,
		)
		if err != nil {
			return nil, fmt.Errorf("parse provider activation context: %w", err)
		}
		record, err := auditdomain.NewProviderActivationRecord(
			activationID,
			providerID,
			gctx,
			startedAt,
			finishedAt,
			policydomain.DecisionKind(policyKind),
			policyReason,
			auditdomain.ApprovalOutcome(approval),
			auditdomain.ProviderActivationStatus(status),
			auditdomain.ProviderActivationErrorClass(errorClass),
		)
		if err != nil {
			return nil, fmt.Errorf("parse provider activation audit: %w", err)
		}
		records = append(records, record.WithErrorSummary(errorSummary))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list provider activation audit rows: %w", err)
	}
	return records, nil
}
