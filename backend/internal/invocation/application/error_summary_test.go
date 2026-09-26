package application

import (
	"errors"
	"testing"
)

type testDiagnosticError struct {
	message string
}

func (e testDiagnosticError) Error() string {
	return "diagnostic failure"
}

func (e testDiagnosticError) DiagnosticSummary() string {
	return e.message
}

func TestAuditErrorSummaryRedactsCommonSecretForms(t *testing.T) {
	t.Parallel()

	err := testDiagnosticError{
		message: "Authorization: Bearer abc.def token=plain as_sk_supersecretvalue",
	}
	got := auditErrorSummary(err)
	if got != "Authorization=[REDACTED] token=[REDACTED] [REDACTED]" {
		t.Fatalf("auditErrorSummary() = %q", got)
	}
}

func TestAuditErrorSummaryTraversesWrappedErrors(t *testing.T) {
	t.Parallel()

	err := errors.Join(errors.New("outer"), testDiagnosticError{message: "upstream rejected input"})
	if got := auditErrorSummary(err); got != "upstream rejected input" {
		t.Fatalf("auditErrorSummary() = %q", got)
	}
}

func TestAuditErrorSummaryIgnoresOrdinaryErrors(t *testing.T) {
	t.Parallel()

	if got := auditErrorSummary(errors.New("internal path /tmp/secret")); got != "" {
		t.Fatalf("auditErrorSummary() = %q, want empty", got)
	}
}
