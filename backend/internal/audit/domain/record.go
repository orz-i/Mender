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
	invocationID invocationdomain.InvocationID
	toolID       catalogdomain.ToolID
	providerID   *catalogdomain.ProviderID
	caller       string
	startedAt    time.Time
	finishedAt   time.Time
	policyKind   policydomain.DecisionKind
	policyReason string
	approval     ApprovalOutcome
	status       Status
	resultKind   invocationdomain.ResultKind
	errorClass   ErrorClass
	errorSummary string
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

func (r Record) InvocationID() invocationdomain.InvocationID { return r.invocationID }
func (r Record) ToolID() catalogdomain.ToolID                { return r.toolID }
func (r Record) ProviderID() *catalogdomain.ProviderID       { return cloneProviderID(r.providerID) }
func (r Record) Caller() string                              { return r.caller }
func (r Record) StartedAt() time.Time                        { return r.startedAt }
func (r Record) FinishedAt() time.Time                       { return r.finishedAt }
func (r Record) PolicyKind() policydomain.DecisionKind       { return r.policyKind }
func (r Record) PolicyReason() string                        { return r.policyReason }
func (r Record) Approval() ApprovalOutcome                   { return r.approval }
func (r Record) Status() Status                              { return r.status }
func (r Record) ResultKind() invocationdomain.ResultKind     { return r.resultKind }
func (r Record) ErrorClass() ErrorClass                      { return r.errorClass }
func (r Record) ErrorSummary() string                        { return r.errorSummary }

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
