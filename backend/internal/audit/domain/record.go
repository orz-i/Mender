package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// Status is an invocation's audited terminal state.
type Status string

const (
	StatusCompleted     Status = "completed"
	StatusInputRequired Status = "input_required"
	StatusTask          Status = "task"
	StatusDenied        Status = "denied"
	StatusFailed        Status = "failed"
	StatusCancelled     Status = "cancelled"
	StatusTimedOut      Status = "timed_out"
)

// ApprovalOutcome records whether an approval gate participated.
type ApprovalOutcome string

const (
	ApprovalNotRequired ApprovalOutcome = "not_required"
	ApprovalApproved    ApprovalOutcome = "approved"
	ApprovalDenied      ApprovalOutcome = "denied"
)

// ErrorClass is a stable non-sensitive failure classification.
type ErrorClass string

const (
	ErrorNone             ErrorClass = ""
	ErrorToolNotFound     ErrorClass = "tool_not_found"
	ErrorInvalidArguments ErrorClass = "invalid_arguments"
	ErrorPolicy           ErrorClass = "policy"
	ErrorApproval         ErrorClass = "approval"
	ErrorExecutorNotFound ErrorClass = "executor_not_found"
	ErrorExecution        ErrorClass = "execution"
	ErrorResultGuard      ErrorClass = "result_guard"
	ErrorCancelled        ErrorClass = "cancelled"
	ErrorTimedOut         ErrorClass = "timed_out"
	ErrorInternal         ErrorClass = "internal"
)

// Record is the data-minimized terminal audit record for one Invocation.
type Record struct {
	invocationID   invocationdomain.InvocationID
	toolID         catalogdomain.ToolID
	providerID     *catalogdomain.ProviderID
	caller         string
	startedAt      time.Time
	finishedAt     time.Time
	policyKind     policydomain.DecisionKind
	policyReason   string
	approval       ApprovalOutcome
	status         Status
	resultKind     invocationdomain.ResultKind
	errorClass     ErrorClass
	errorSummary   string
	origin         invocationdomain.Origin
	scopeKind      invocationdomain.ScopeKind
	scopeReference string
	outcomeKind    invocationdomain.OutcomeKind
	rejectionCode  invocationdomain.RejectionCode
	failureSource  invocationdomain.FailureSource
	failureCode    invocationdomain.FailureCode
}

const maxErrorSummaryRunes = 512

// NewRecord constructs a terminal audit record.
func NewRecord(
	invocationID invocationdomain.InvocationID,
	toolID catalogdomain.ToolID,
	providerID *catalogdomain.ProviderID,
	caller string,
	startedAt time.Time,
	finishedAt time.Time,
	policyKind policydomain.DecisionKind,
	policyReason string,
	approval ApprovalOutcome,
	status Status,
	resultKind invocationdomain.ResultKind,
	errorClass ErrorClass,
) (Record, error) {
	if _, err := invocationdomain.ParseInvocationID(invocationID.String()); err != nil {
		return Record{}, err
	}
	if _, err := catalogdomain.ParseToolID(toolID.String()); err != nil {
		return Record{}, err
	}
	if providerID != nil {
		if _, err := catalogdomain.ParseProviderID(providerID.String()); err != nil {
			return Record{}, err
		}
	}
	if startedAt.IsZero() || finishedAt.IsZero() {
		return Record{}, fmt.Errorf("audit timestamps must not be zero")
	}
	if finishedAt.Before(startedAt) {
		return Record{}, fmt.Errorf("audit finished_at must not be before started_at")
	}
	if len(strings.TrimSpace(caller)) > 255 {
		return Record{}, fmt.Errorf("audit caller is too long")
	}
	if err := validateOutcome(status, resultKind, errorClass); err != nil {
		return Record{}, err
	}

	return Record{
		invocationID: invocationID,
		toolID:       toolID,
		providerID:   cloneProviderID(providerID),
		caller:       strings.TrimSpace(caller),
		startedAt:    startedAt.UTC(),
		finishedAt:   finishedAt.UTC(),
		policyKind:   policyKind,
		policyReason: policyReason,
		approval:     approval,
		status:       status,
		resultKind:   resultKind,
		errorClass:   errorClass,
	}, nil
}

func validateOutcome(status Status, resultKind invocationdomain.ResultKind, errorClass ErrorClass) error {
	switch status {
	case StatusCompleted:
		if resultKind != invocationdomain.ResultKindComplete || errorClass != ErrorNone {
			return fmt.Errorf("completed audit outcome is inconsistent")
		}
	case StatusInputRequired:
		if resultKind != invocationdomain.ResultKindInputRequired || errorClass != ErrorNone {
			return fmt.Errorf("input_required audit outcome is inconsistent")
		}
	case StatusTask:
		if resultKind != invocationdomain.ResultKindTask || errorClass != ErrorNone {
			return fmt.Errorf("task audit outcome is inconsistent")
		}
	case StatusDenied, StatusFailed, StatusCancelled, StatusTimedOut:
		if resultKind != "" || errorClass == ErrorNone {
			return fmt.Errorf("%s audit outcome requires error class and no result kind", status)
		}
	default:
		return fmt.Errorf("unknown audit status %q", status)
	}
	return nil
}

func (r Record) InvocationID() invocationdomain.InvocationID   { return r.invocationID }
func (r Record) ToolID() catalogdomain.ToolID                  { return r.toolID }
func (r Record) ProviderID() *catalogdomain.ProviderID         { return cloneProviderID(r.providerID) }
func (r Record) Caller() string                                { return r.caller }
func (r Record) StartedAt() time.Time                          { return r.startedAt }
func (r Record) FinishedAt() time.Time                         { return r.finishedAt }
func (r Record) PolicyKind() policydomain.DecisionKind         { return r.policyKind }
func (r Record) PolicyReason() string                          { return r.policyReason }
func (r Record) Approval() ApprovalOutcome                     { return r.approval }
func (r Record) Status() Status                                { return r.status }
func (r Record) ResultKind() invocationdomain.ResultKind       { return r.resultKind }
func (r Record) ErrorClass() ErrorClass                        { return r.errorClass }
func (r Record) ErrorSummary() string                          { return r.errorSummary }
func (r Record) Origin() invocationdomain.Origin               { return r.origin }
func (r Record) ScopeKind() invocationdomain.ScopeKind         { return r.scopeKind }
func (r Record) ScopeReference() string                        { return r.scopeReference }
func (r Record) OutcomeKind() invocationdomain.OutcomeKind     { return r.outcomeKind }
func (r Record) RejectionCode() invocationdomain.RejectionCode { return r.rejectionCode }
func (r Record) FailureSource() invocationdomain.FailureSource { return r.failureSource }
func (r Record) FailureCode() invocationdomain.FailureCode     { return r.failureCode }

// WithExecutionContext attaches non-secret execution scope/origin metadata.
func (r Record) WithExecutionContext(
	executionContext invocationdomain.InvocationContext,
) (Record, error) {
	if err := executionContext.Validate(); err != nil {
		return Record{}, fmt.Errorf("audit execution context: %w", err)
	}
	scope := executionContext.Scope()
	r.origin = executionContext.Origin()
	r.scopeKind = scope.Kind()
	r.scopeReference = scope.Reference()
	return r, nil
}

// WithOutcome attaches canonical outcome classification without retaining payload content.
func (r Record) WithOutcome(outcome invocationdomain.Outcome) (Record, error) {
	if outcome.Kind() == "" {
		return Record{}, fmt.Errorf("audit outcome metadata requires a canonical outcome")
	}
	return r.withOutcomeMetadata(
		outcome.Kind(),
		outcome.RejectionCode(),
		outcome.FailureSource(),
		outcome.FailureCode(),
	)
}

// WithOutcomeClassification attaches already-normalized canonical outcome metadata.
func (r Record) WithOutcomeClassification(
	outcomeKind invocationdomain.OutcomeKind,
	rejectionCode invocationdomain.RejectionCode,
	failureSource invocationdomain.FailureSource,
	failureCode invocationdomain.FailureCode,
) (Record, error) {
	return r.withOutcomeMetadata(outcomeKind, rejectionCode, failureSource, failureCode)
}

// RestoreGovernanceMetadata restores persisted Phase 9 metadata.
// All-zero metadata is accepted for legacy pre-Phase-9 Audit rows.
func (r Record) RestoreGovernanceMetadata(
	origin invocationdomain.Origin,
	scopeKind invocationdomain.ScopeKind,
	scopeReference string,
	outcomeKind invocationdomain.OutcomeKind,
	rejectionCode invocationdomain.RejectionCode,
	failureSource invocationdomain.FailureSource,
	failureCode invocationdomain.FailureCode,
) (Record, error) {
	if origin == "" &&
		scopeKind == "" &&
		scopeReference == "" &&
		outcomeKind == "" &&
		rejectionCode == "" &&
		failureSource == "" &&
		failureCode == "" {
		return r, nil
	}

	if origin == "" || scopeKind == "" {
		return Record{}, fmt.Errorf("audit governance metadata requires origin and scope")
	}

	var scope invocationdomain.ExecutionScope
	switch scopeKind {
	case invocationdomain.ScopeCatalog:
		if scopeReference != "" {
			return Record{}, fmt.Errorf("audit catalog scope must not contain a reference")
		}
		scope = invocationdomain.CatalogExecutionScope()
	case invocationdomain.ScopeProfile:
		var err error
		scope, err = invocationdomain.ProfileExecutionScope(scopeReference)
		if err != nil {
			return Record{}, fmt.Errorf("audit profile scope: %w", err)
		}
	default:
		return Record{}, fmt.Errorf("audit scope kind %q is invalid", scopeKind)
	}

	executionContext, err := invocationdomain.NewInvocationContext(origin, r.caller, scope)
	if err != nil {
		return Record{}, fmt.Errorf("audit execution context: %w", err)
	}
	r, err = r.WithExecutionContext(executionContext)
	if err != nil {
		return Record{}, err
	}

	if outcomeKind == "" {
		if rejectionCode != "" || failureSource != "" || failureCode != "" {
			return Record{}, fmt.Errorf("audit empty outcome kind cannot contain outcome classification")
		}
		return r, nil
	}

	return r.withOutcomeMetadata(outcomeKind, rejectionCode, failureSource, failureCode)
}

func (r Record) withOutcomeMetadata(
	outcomeKind invocationdomain.OutcomeKind,
	rejectionCode invocationdomain.RejectionCode,
	failureSource invocationdomain.FailureSource,
	failureCode invocationdomain.FailureCode,
) (Record, error) {
	switch outcomeKind {
	case invocationdomain.OutcomeKindComplete,
		invocationdomain.OutcomeKindInputRequired,
		invocationdomain.OutcomeKindTask:
		if rejectionCode != "" || failureSource != "" || failureCode != "" {
			return Record{}, fmt.Errorf("%s audit outcome contains incompatible classification", outcomeKind)
		}
	case invocationdomain.OutcomeKindRejected:
		switch rejectionCode {
		case invocationdomain.RejectionToolNotFound,
			invocationdomain.RejectionToolOutsideScope,
			invocationdomain.RejectionInvalidArguments,
			invocationdomain.RejectionPolicyDenied,
			invocationdomain.RejectionApprovalDenied:
		default:
			return Record{}, fmt.Errorf("audit rejection code %q is invalid", rejectionCode)
		}
		if failureSource != "" || failureCode != "" {
			return Record{}, fmt.Errorf("rejected audit outcome contains failure classification")
		}
	case invocationdomain.OutcomeKindFailed:
		switch failureSource {
		case invocationdomain.FailureSourceTool:
			if failureCode != invocationdomain.FailureToolError {
				return Record{}, fmt.Errorf("audit Tool failure code %q is invalid", failureCode)
			}
		case invocationdomain.FailureSourceProvider:
			if failureCode != invocationdomain.FailureProviderError {
				return Record{}, fmt.Errorf("audit Provider failure code %q is invalid", failureCode)
			}
		case invocationdomain.FailureSourceRuntime:
			switch failureCode {
			case invocationdomain.FailureExecutorUnavailable,
				invocationdomain.FailureCredentialOutsideScope,
				invocationdomain.FailureTimedOut,
				invocationdomain.FailureCancelled:
			default:
				return Record{}, fmt.Errorf("audit Runtime failure code %q is invalid", failureCode)
			}
		case invocationdomain.FailureSourceResultGuard:
			switch failureCode {
			case invocationdomain.FailureOutputSchemaMismatch,
				invocationdomain.FailureResultTooLarge:
			default:
				return Record{}, fmt.Errorf("audit Result Guard failure code %q is invalid", failureCode)
			}
		default:
			return Record{}, fmt.Errorf("audit failure source %q is invalid", failureSource)
		}
		if rejectionCode != "" {
			return Record{}, fmt.Errorf("failed audit outcome contains rejection classification")
		}
	default:
		return Record{}, fmt.Errorf("audit outcome kind %q is invalid", outcomeKind)
	}

	if err := validateGovernanceOutcomeConsistency(
		r.status,
		r.resultKind,
		r.errorClass,
		outcomeKind,
		rejectionCode,
		failureSource,
		failureCode,
	); err != nil {
		return Record{}, err
	}

	r.outcomeKind = outcomeKind
	r.rejectionCode = rejectionCode
	r.failureSource = failureSource
	r.failureCode = failureCode
	return r, nil
}

func validateGovernanceOutcomeConsistency(
	status Status,
	resultKind invocationdomain.ResultKind,
	errorClass ErrorClass,
	outcomeKind invocationdomain.OutcomeKind,
	rejectionCode invocationdomain.RejectionCode,
	failureSource invocationdomain.FailureSource,
	failureCode invocationdomain.FailureCode,
) error {
	switch outcomeKind {
	case invocationdomain.OutcomeKindComplete:
		if status != StatusCompleted || resultKind != invocationdomain.ResultKindComplete || errorClass != ErrorNone {
			return fmt.Errorf("complete Audit outcome metadata is inconsistent")
		}
	case invocationdomain.OutcomeKindInputRequired:
		if status != StatusInputRequired || resultKind != invocationdomain.ResultKindInputRequired || errorClass != ErrorNone {
			return fmt.Errorf("input_required Audit outcome metadata is inconsistent")
		}
	case invocationdomain.OutcomeKindTask:
		if status != StatusTask || resultKind != invocationdomain.ResultKindTask || errorClass != ErrorNone {
			return fmt.Errorf("task Audit outcome metadata is inconsistent")
		}
	case invocationdomain.OutcomeKindRejected:
		if resultKind != "" {
			return fmt.Errorf("rejected Audit outcome metadata must not have result kind")
		}
		switch rejectionCode {
		case invocationdomain.RejectionToolNotFound:
			if status != StatusFailed || errorClass != ErrorToolNotFound {
				return fmt.Errorf("tool_not_found Audit rejection metadata is inconsistent")
			}
		case invocationdomain.RejectionToolOutsideScope, invocationdomain.RejectionPolicyDenied:
			if status != StatusDenied || errorClass != ErrorPolicy {
				return fmt.Errorf("policy Audit rejection metadata is inconsistent")
			}
		case invocationdomain.RejectionInvalidArguments:
			if status != StatusFailed || errorClass != ErrorInvalidArguments {
				return fmt.Errorf("invalid_arguments Audit rejection metadata is inconsistent")
			}
		case invocationdomain.RejectionApprovalDenied:
			if status != StatusDenied || errorClass != ErrorApproval {
				return fmt.Errorf("approval Audit rejection metadata is inconsistent")
			}
		}
	case invocationdomain.OutcomeKindFailed:
		if resultKind != "" {
			return fmt.Errorf("failed Audit outcome metadata must not have result kind")
		}
		switch failureSource {
		case invocationdomain.FailureSourceTool, invocationdomain.FailureSourceProvider:
			if status != StatusFailed || errorClass != ErrorExecution {
				return fmt.Errorf("Tool/Provider Audit failure metadata is inconsistent")
			}
		case invocationdomain.FailureSourceRuntime:
			switch failureCode {
			case invocationdomain.FailureExecutorUnavailable:
				if status != StatusFailed || errorClass != ErrorExecutorNotFound {
					return fmt.Errorf("executor_unavailable Audit failure metadata is inconsistent")
				}
			case invocationdomain.FailureCredentialOutsideScope:
				if status != StatusFailed || errorClass != ErrorPolicy {
					return fmt.Errorf("credential_outside_scope Audit failure metadata is inconsistent")
				}
			case invocationdomain.FailureTimedOut:
				if status != StatusTimedOut || errorClass != ErrorTimedOut {
					return fmt.Errorf("timed_out Audit failure metadata is inconsistent")
				}
			case invocationdomain.FailureCancelled:
				if status != StatusCancelled || errorClass != ErrorCancelled {
					return fmt.Errorf("cancelled Audit failure metadata is inconsistent")
				}
			}
		case invocationdomain.FailureSourceResultGuard:
			if status != StatusFailed || errorClass != ErrorResultGuard {
				return fmt.Errorf("result_guard Audit failure metadata is inconsistent")
			}
		}
	}
	return nil
}

// WithErrorSummary returns the same terminal Audit record with a bounded,
// single-line diagnostic summary. The summary is metadata only; callers must
// redact secret material before passing it into the Audit Domain.
func (r Record) WithErrorSummary(summary string) Record {
	r.errorSummary = normalizeErrorSummary(summary)
	return r
}

func normalizeErrorSummary(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))
	lastSpace := false
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	normalized := strings.TrimSpace(b.String())
	runes := []rune(normalized)
	if len(runes) > maxErrorSummaryRunes {
		normalized = string(runes[:maxErrorSummaryRunes-1]) + "…"
	}
	return normalized
}

func cloneProviderID(id *catalogdomain.ProviderID) *catalogdomain.ProviderID {
	if id == nil {
		return nil
	}
	cloned := *id
	return &cloned
}
