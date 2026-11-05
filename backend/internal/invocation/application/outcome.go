package application

import (
	"context"
	"errors"
	"fmt"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func isExecutionContextError(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func newRejectedOutcome(code invocationdomain.RejectionCode, message string) (invocationdomain.Outcome, error) {
	outcome, err := invocationdomain.Rejected(code, message)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("invoke tool: construct rejection outcome: %w", err)
	}
	return outcome, nil
}

func newFailedOutcome(source invocationdomain.FailureSource, code invocationdomain.FailureCode, message string) (invocationdomain.Outcome, error) {
	outcome, err := invocationdomain.Failed(source, code, message)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("invoke tool: construct failure outcome: %w", err)
	}
	return outcome, nil
}

func failedOutcomeForExecutionError(ctx context.Context, err error) (invocationdomain.Outcome, auditdomain.Status, auditdomain.ErrorClass, error) {
	status, class := classifyExecutionError(ctx, err)
	source := invocationdomain.FailureSourceProvider
	code := invocationdomain.FailureProviderError
	message := "provider execution failed"
	switch status {
	case auditdomain.StatusTimedOut:
		source, code, message = invocationdomain.FailureSourceRuntime, invocationdomain.FailureTimedOut, "tool execution timed out"
	case auditdomain.StatusCancelled:
		source, code, message = invocationdomain.FailureSourceRuntime, invocationdomain.FailureCancelled, "tool execution was cancelled"
	default:
		if errors.Is(err, ErrCredentialOutsideScope) {
			source, code, message = invocationdomain.FailureSourceRuntime, invocationdomain.FailureCredentialOutsideScope, "credential is not available in this execution scope"
		} else if summary := auditErrorSummary(err); summary != "" {
			message = summary
		}
	}
	outcome, outcomeErr := newFailedOutcome(source, code, message)
	return outcome, status, class, outcomeErr
}

func auditClassificationForExecutorOutcome(outcome invocationdomain.Outcome) (auditdomain.Status, invocationdomain.ResultKind, auditdomain.ErrorClass, bool) {
	switch outcome.Kind() {
	case invocationdomain.OutcomeKindComplete:
		return auditdomain.StatusCompleted, invocationdomain.ResultKindComplete, auditdomain.ErrorNone, true
	case invocationdomain.OutcomeKindInputRequired:
		return auditdomain.StatusInputRequired, invocationdomain.ResultKindInputRequired, auditdomain.ErrorNone, true
	case invocationdomain.OutcomeKindTask:
		return auditdomain.StatusTask, invocationdomain.ResultKindTask, auditdomain.ErrorNone, true
	case invocationdomain.OutcomeKindFailed:
		switch outcome.FailureSource() {
		case invocationdomain.FailureSourceTool, invocationdomain.FailureSourceProvider:
			return auditdomain.StatusFailed, "", auditdomain.ErrorExecution, true
		case invocationdomain.FailureSourceRuntime:
			switch outcome.FailureCode() {
			case invocationdomain.FailureExecutorUnavailable:
				return auditdomain.StatusFailed, "", auditdomain.ErrorExecutorNotFound, true
			case invocationdomain.FailureCredentialOutsideScope:
				return auditdomain.StatusFailed, "", auditdomain.ErrorPolicy, true
			case invocationdomain.FailureTimedOut:
				return auditdomain.StatusTimedOut, "", auditdomain.ErrorTimedOut, true
			case invocationdomain.FailureCancelled:
				return auditdomain.StatusCancelled, "", auditdomain.ErrorCancelled, true
			default:
				return "", "", "", false
			}
		case invocationdomain.FailureSourceResultGuard:
			return auditdomain.StatusFailed, "", auditdomain.ErrorResultGuard, true
		default:
			return "", "", "", false
		}
	default:
		return "", "", "", false
	}
}

func classifyExecutionError(ctx context.Context, err error) (auditdomain.Status, auditdomain.ErrorClass) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		return auditdomain.StatusTimedOut, auditdomain.ErrorTimedOut
	case errors.Is(err, context.Canceled), errors.Is(ctx.Err(), context.Canceled):
		return auditdomain.StatusCancelled, auditdomain.ErrorCancelled
	case errors.Is(err, ErrCredentialOutsideScope):
		return auditdomain.StatusFailed, auditdomain.ErrorPolicy
	default:
		return auditdomain.StatusFailed, auditdomain.ErrorExecution
	}
}
