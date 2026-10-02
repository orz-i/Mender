package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxOutcomeMessageRunes = 2048

// OutcomeKind identifies the protocol-independent result of a governed Invocation.
type OutcomeKind string

const (
	OutcomeKindComplete      OutcomeKind = "complete"
	OutcomeKindInputRequired OutcomeKind = "input_required"
	OutcomeKindTask          OutcomeKind = "task"
	OutcomeKindRejected      OutcomeKind = "rejected"
	OutcomeKindFailed        OutcomeKind = "failed"
)

// RejectionCode identifies a stable pre-execution rejection reason.
type RejectionCode string

const (
	RejectionToolNotFound          RejectionCode = "tool_not_found"
	RejectionToolOutsideScope      RejectionCode = "tool_outside_scope"
	RejectionInvalidArguments      RejectionCode = "invalid_arguments"
	RejectionPolicyDenied          RejectionCode = "policy_denied"
	RejectionApprovalDenied        RejectionCode = "approval_denied"
	RejectionRuntimeBudgetExceeded RejectionCode = "runtime_budget_exceeded"
)

// FailureSource identifies which governed execution boundary failed.
type FailureSource string

const (
	FailureSourceTool        FailureSource = "tool"
	FailureSourceProvider    FailureSource = "provider"
	FailureSourceRuntime     FailureSource = "runtime"
	FailureSourceResultGuard FailureSource = "result_guard"
)

// FailureCode identifies a stable operational failure class.
type FailureCode string

const (
	FailureToolError              FailureCode = "tool_error"
	FailureProviderError          FailureCode = "provider_error"
	FailureExecutorUnavailable    FailureCode = "executor_unavailable"
	FailureCredentialOutsideScope FailureCode = "credential_outside_scope" // #nosec G101 -- stable failure classification, not a credential value
	FailureTimedOut               FailureCode = "timed_out"
	FailureCancelled              FailureCode = "cancelled"
	FailureOutputSchemaMismatch   FailureCode = "output_schema_mismatch"
	FailureResultTooLarge         FailureCode = "result_too_large"
)

// Outcome is the canonical, protocol-independent result of one governed Invocation.
type Outcome struct {
	kind        OutcomeKind
	content     JSONValue
	requestID   string
	message     string
	inputSchema []byte
	taskID      string
	taskStatus  string
	rejectCode  RejectionCode
	failSource  FailureSource
	failCode    FailureCode
}

// Complete constructs an immediately completed outcome.
func Complete(content JSONValue) (Outcome, error) {
	if content.IsZero() {
		return Outcome{}, validationError("content", "must be provided")
	}
	return Outcome{kind: OutcomeKindComplete, content: content}, nil
}

// InputRequired constructs an outcome requesting additional caller/model input.
func InputRequired(requestID, message string, schema []byte) (Outcome, error) {
	requestID = strings.TrimSpace(requestID)
	message = strings.TrimSpace(message)
	if requestID == "" {
		return Outcome{}, validationError("request_id", "must not be empty")
	}
	if message == "" {
		return Outcome{}, validationError("message", "must not be empty")
	}
	if len(schema) > 0 {
		if _, err := validateSchema(schema); err != nil {
			return Outcome{}, err
		}
	}

	return Outcome{
		kind:        OutcomeKindInputRequired,
		requestID:   requestID,
		message:     message,
		inputSchema: cloneBytes(schema),
	}, nil
}

// Task constructs an asynchronous task handle outcome.
func Task(taskID, status, message string) (Outcome, error) {
	taskID = strings.TrimSpace(taskID)
	status = strings.TrimSpace(status)
	if taskID == "" {
		return Outcome{}, validationError("task_id", "must not be empty")
	}
	if status == "" {
		return Outcome{}, validationError("task_status", "must not be empty")
	}

	return Outcome{
		kind:       OutcomeKindTask,
		taskID:     taskID,
		taskStatus: status,
		message:    strings.TrimSpace(message),
	}, nil
}

// Rejected constructs an expected pre-execution rejection.
func Rejected(code RejectionCode, message string) (Outcome, error) {
	if !validRejectionCode(code) {
		return Outcome{}, validationError("rejection_code", fmt.Sprintf("unknown code %q", code))
	}
	message, err := normalizeOutcomeMessage(message)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		kind:       OutcomeKindRejected,
		rejectCode: code,
		message:    message,
	}, nil
}

// Failed constructs an expected operational failure.
func Failed(source FailureSource, code FailureCode, message string) (Outcome, error) {
	if !validFailureSource(source) {
		return Outcome{}, validationError("failure_source", fmt.Sprintf("unknown source %q", source))
	}
	if !validFailureCode(source, code) {
		return Outcome{}, validationError(
			"failure_code",
			fmt.Sprintf("code %q is invalid for source %q", code, source),
		)
	}
	message, err := normalizeOutcomeMessage(message)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		kind:       OutcomeKindFailed,
		failSource: source,
		failCode:   code,
		message:    message,
	}, nil
}

func normalizeOutcomeMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", validationError("message", "must not be empty")
	}
	if utf8.RuneCountInString(message) > maxOutcomeMessageRunes {
		return "", validationError("message", "is too long")
	}
	return message, nil
}

func validRejectionCode(code RejectionCode) bool {
	switch code {
	case RejectionToolNotFound,
		RejectionToolOutsideScope,
		RejectionInvalidArguments,
		RejectionPolicyDenied,
		RejectionApprovalDenied,
		RejectionRuntimeBudgetExceeded:
		return true
	default:
		return false
	}
}

func validFailureSource(source FailureSource) bool {
	switch source {
	case FailureSourceTool, FailureSourceProvider, FailureSourceRuntime, FailureSourceResultGuard:
		return true
	default:
		return false
	}
}

func validFailureCode(source FailureSource, code FailureCode) bool {
	switch source {
	case FailureSourceTool:
		return code == FailureToolError
	case FailureSourceProvider:
		return code == FailureProviderError
	case FailureSourceRuntime:
		return code == FailureExecutorUnavailable ||
			code == FailureCredentialOutsideScope ||
			code == FailureTimedOut ||
			code == FailureCancelled
	case FailureSourceResultGuard:
		return code == FailureOutputSchemaMismatch || code == FailureResultTooLarge
	default:
		return false
	}
}

func (o Outcome) Kind() OutcomeKind {
	return o.kind
}

func (o Outcome) Content() JSONValue {
	return o.content
}

func (o Outcome) RequestID() string {
	return o.requestID
}

func (o Outcome) Message() string {
	return o.message
}

func (o Outcome) InputSchema() []byte {
	return cloneBytes(o.inputSchema)
}

func (o Outcome) TaskID() string {
	return o.taskID
}

func (o Outcome) TaskStatus() string {
	return o.taskStatus
}

func (o Outcome) RejectionCode() RejectionCode {
	return o.rejectCode
}

func (o Outcome) FailureSource() FailureSource {
	return o.failSource
}

func (o Outcome) FailureCode() FailureCode {
	return o.failCode
}

// Result and ResultKind remain aliases during Phase 9 migration so Audit and
// executor implementations can move to Outcome without a schema migration in
// the same commit.
type Result = Outcome
type ResultKind = OutcomeKind

const (
	ResultKindComplete      = OutcomeKindComplete
	ResultKindInputRequired = OutcomeKindInputRequired
	ResultKindTask          = OutcomeKindTask
)
