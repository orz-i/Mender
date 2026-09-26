package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// Record persists one terminal Audit record.
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
			caller,
			started_at,
			finished_at,
			policy_kind,
			policy_reason,
			approval,
			status,
			result_kind,
			error_class
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.InvocationID().String(),
		record.ToolID().String(),
		providerID,
		record.Caller(),
		formatTime(record.StartedAt()),
		formatTime(record.FinishedAt()),
		string(record.PolicyKind()),
		record.PolicyReason(),
		string(record.Approval()),
		string(record.Status()),
		string(record.ResultKind()),
		string(record.ErrorClass()),
	)
	if err != nil {
		return fmt.Errorf("record invocation audit: %w", err)
	}
	return nil
}

// Get returns one Audit record by InvocationID.
//
// It is currently used by infrastructure tests and future management queries.
func (s *Store) Get(
	ctx context.Context,
	id invocationdomain.InvocationID,
) (auditdomain.Record, error) {
	var (
		rawInvocationID string
		rawToolID       string
		rawProviderID   sql.NullString
		caller          string
		rawStarted      string
		rawFinished     string
		policyKind      string
		policyReason    string
		approval        string
		status          string
		resultKind      string
		errorClass      string
	)

	err := s.db.QueryRowContext(
		ctx,
		`SELECT
			invocation_id,
			tool_id,
			provider_id,
			caller,
			started_at,
			finished_at,
			policy_kind,
			policy_reason,
			approval,
			status,
			result_kind,
			error_class
		FROM invocation_audit
		WHERE invocation_id = ?`,
		id.String(),
	).Scan(
		&rawInvocationID,
		&rawToolID,
		&rawProviderID,
		&caller,
		&rawStarted,
		&rawFinished,
		&policyKind,
		&policyReason,
		&approval,
		&status,
		&resultKind,
		&errorClass,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return auditdomain.Record{}, sql.ErrNoRows
	}
	if err != nil {
		return auditdomain.Record{}, fmt.Errorf("get invocation audit: %w", err)
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
		caller,
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
		return auditdomain.Record{}, fmt.Errorf("restore invocation audit: %w", err)
	}
	return record, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
