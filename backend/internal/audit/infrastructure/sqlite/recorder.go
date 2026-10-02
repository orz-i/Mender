package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// Record persists one terminal Invocation Audit record.
func (s *Store) Record(ctx context.Context, record auditdomain.Record) error {
	var providerID any
	if id := record.ProviderID(); id != nil {
		providerID = id.String()
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO invocation_audit(
			invocation_id,
			tool_id,
			provider_id,
			principal_kind,
			principal_ref,
			started_at,
			finished_at,
			policy_kind,
			policy_reason,
			approval,
			status,
			result_kind,
			error_class,
			error_summary,
			origin,
			scope_kind,
			scope_reference,
			outcome_kind,
			rejection_code,
			failure_source,
			failure_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.InvocationID().String(),
		record.ToolID().String(),
		providerID,
		record.PrincipalKind(),
		record.PrincipalRef(),
		formatTime(record.StartedAt()),
		formatTime(record.FinishedAt()),
		string(record.PolicyKind()),
		record.PolicyReason(),
		string(record.Approval()),
		string(record.Status()),
		string(record.ResultKind()),
		string(record.ErrorClass()),
		record.ErrorSummary(),
		string(record.Origin()),
		string(record.ScopeKind()),
		record.ScopeReference(),
		string(record.OutcomeKind()),
		string(record.RejectionCode()),
		string(record.FailureSource()),
		string(record.FailureCode()),
	)
	if err != nil {
		return fmt.Errorf("record invocation audit: %w", err)
	}
	return nil
}

func (s *Store) Get(
	ctx context.Context,
	id invocationdomain.InvocationID,
) (auditdomain.Record, error) {
	row := s.db.QueryRowContext(
		ctx,
		auditSelect+`
		WHERE invocation_id = ?`,
		id.String(),
	)
	record, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return auditdomain.Record{}, sql.ErrNoRows
	}
	if err != nil {
		return auditdomain.Record{}, fmt.Errorf("get invocation audit: %w", err)
	}
	return record, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func (s *Store) ListRecords(
	ctx context.Context,
	limit int,
) ([]auditdomain.Record, error) {
	rows, err := s.db.QueryContext(
		ctx,
		auditSelect+`
		ORDER BY finished_at DESC, invocation_id DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list invocation audit: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := make([]auditdomain.Record, 0, limit)
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list invocation audit: %w", scanErr)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list invocation audit: rows: %w", err)
	}
	return records, nil
}

const auditSelect = `SELECT
	invocation_id,
	tool_id,
	provider_id,
	principal_kind,
	principal_ref,
	started_at,
	finished_at,
	policy_kind,
	policy_reason,
	approval,
	status,
	result_kind,
	error_class,
	error_summary,
	origin,
	scope_kind,
	scope_reference,
	outcome_kind,
	rejection_code,
	failure_source,
	failure_code
FROM invocation_audit`

func scanRecord(scanner interface{ Scan(...any) error }) (auditdomain.Record, error) {
	var (
		rawInvocationID string
		rawToolID       string
		rawProviderID   sql.NullString
		principalKind   string
		principalRef    string
		rawStarted      string
		rawFinished     string
		policyKind      string
		policyReason    string
		approval        string
		status          string
		resultKind      string
		errorClass      string
		errorSummary    string
		origin          string
		scopeKind       string
		scopeReference  string
		outcomeKind     string
		rejectionCode   string
		failureSource   string
		failureCode     string
	)
	if err := scanner.Scan(
		&rawInvocationID,
		&rawToolID,
		&rawProviderID,
		&principalKind,
		&principalRef,
		&rawStarted,
		&rawFinished,
		&policyKind,
		&policyReason,
		&approval,
		&status,
		&resultKind,
		&errorClass,
		&errorSummary,
		&origin,
		&scopeKind,
		&scopeReference,
		&outcomeKind,
		&rejectionCode,
		&failureSource,
		&failureCode,
	); err != nil {
		return auditdomain.Record{}, err
	}

	invocationID, err := invocationdomain.ParseInvocationID(rawInvocationID)
	if err != nil {
		return auditdomain.Record{}, fmt.Errorf("parse invocation id: %w", err)
	}
	toolID, err := catalogdomain.ParseToolID(rawToolID)
	if err != nil {
		return auditdomain.Record{}, fmt.Errorf("parse tool id: %w", err)
	}

	var providerID *catalogdomain.ProviderID
	if rawProviderID.Valid {
		id, parseErr := catalogdomain.ParseProviderID(rawProviderID.String)
		if parseErr != nil {
			return auditdomain.Record{}, fmt.Errorf("parse provider id: %w", parseErr)
		}
		providerID = &id
	}

	startedAt, err := time.Parse(time.RFC3339Nano, rawStarted)
	if err != nil {
		return auditdomain.Record{}, fmt.Errorf("parse started_at: %w", err)
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, rawFinished)
	if err != nil {
		return auditdomain.Record{}, fmt.Errorf("parse finished_at: %w", err)
	}

	record, err := auditdomain.NewRecord(
		invocationID,
		toolID,
		providerID,
		startedAt,
		finishedAt,
		policydomain.DecisionKind(policyKind),
		policyReason,
		auditdomain.ApprovalOutcome(approval),
		auditdomain.Status(status),
		invocationdomain.ResultKind(resultKind),
		auditdomain.ErrorClass(errorClass),
	)
	if err != nil {
		return auditdomain.Record{}, err
	}
	record = record.WithErrorSummary(errorSummary)
	record, err = record.RestoreGovernanceMetadata(
		principalKind,
		principalRef,
		governancedomain.Origin(origin),
		governancedomain.ScopeKind(scopeKind),
		scopeReference,
		invocationdomain.OutcomeKind(outcomeKind),
		invocationdomain.RejectionCode(rejectionCode),
		invocationdomain.FailureSource(failureSource),
		invocationdomain.FailureCode(failureCode),
	)
	if err != nil {
		return auditdomain.Record{}, err
	}
	return record, nil
}
