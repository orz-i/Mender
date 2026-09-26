package application

import "errors"

var (
	// ErrPolicyDenied indicates that policy explicitly rejected execution.
	ErrPolicyDenied = errors.New("invocation: denied by policy")
	// ErrApprovalDenied indicates that required human/external approval was rejected.
	ErrApprovalDenied = errors.New("invocation: approval denied")
	// ErrExecutorNotFound indicates that no executor is registered for the Tool.
	ErrExecutorNotFound = errors.New("invocation: executor not found")
	// ErrInvalidTimeout indicates an explicitly supplied non-positive timeout.
	ErrInvalidTimeout = errors.New("invocation: timeout must be positive")
	// ErrInvalidExecutorResult indicates an executor returned no canonical result variant.
	ErrInvalidExecutorResult = errors.New("invocation: executor returned invalid result")
	// ErrAuditFailed indicates that the terminal outcome could not be recorded.
	ErrAuditFailed = errors.New("invocation: audit record failed")
)
