package domain

import (
	"strings"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestRecordContainsOnlyMetadataFields(t *testing.T) {
	t.Parallel()

	invocationID, _ := invocationdomain.ParseInvocationID("inv_00000000000000000000000000000001")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	record, err := NewRecord(
		invocationID,
		toolID,
		&providerID,
		now,
		now.Add(time.Second),
		policydomain.DecisionAllow,
		"local.default",
		ApprovalNotRequired,
		StatusCompleted,
		invocationdomain.ResultKindComplete,
		ErrorNone,
	)
	if err != nil {
		t.Fatalf("NewRecord() error = %v", err)
	}

	if record.InvocationID() != invocationID || record.ToolID() != toolID || record.Status() != StatusCompleted {
		t.Fatalf("record = %#v", record)
	}
}

func TestRecordErrorSummaryIsBoundedAndSingleLine(t *testing.T) {
	t.Parallel()

	invocationID, _ := invocationdomain.ParseInvocationID("inv_00000000000000000000000000000002")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000002")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000002")
	now := time.Now().UTC()

	record, err := NewRecord(
		invocationID,
		toolID,
		&providerID,
		now,
		now,
		policydomain.DecisionAllow,
		"local.default",
		ApprovalNotRequired,
		StatusFailed,
		"",
		ErrorExecution,
	)
	if err != nil {
		t.Fatal(err)
	}

	record = record.WithErrorSummary(" first\n\tsecond  " + strings.Repeat("x", 600))
	if strings.ContainsAny(record.ErrorSummary(), "\n\t") {
		t.Fatalf("ErrorSummary() contains controls: %q", record.ErrorSummary())
	}
	if len([]rune(record.ErrorSummary())) != maxErrorSummaryRunes {
		t.Fatalf("ErrorSummary() rune length = %d", len([]rune(record.ErrorSummary())))
	}
	if !strings.HasPrefix(record.ErrorSummary(), "first second ") {
		t.Fatalf("ErrorSummary() = %q", record.ErrorSummary())
	}
}

func TestRecordRejectsInconsistentGovernanceOutcomeMetadata(t *testing.T) {
	t.Parallel()

	invocationID, _ := invocationdomain.ParseInvocationID("inv_00000000000000000000000000000004")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000004")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000004")
	now := time.Now().UTC()

	record, err := NewRecord(
		invocationID,
		toolID,
		&providerID,
		now,
		now,
		policydomain.DecisionAllow,
		"local.default",
		ApprovalNotRequired,
		StatusCompleted,
		invocationdomain.ResultKindComplete,
		ErrorNone,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = record.WithOutcomeClassification(
		invocationdomain.OutcomeKindFailed,
		"",
		invocationdomain.FailureSourceProvider,
		invocationdomain.FailureProviderError,
	)
	if err == nil {
		t.Fatal("WithOutcomeClassification() accepted failure metadata for completed record")
	}
}

func TestRecordRejectsInconsistentOutcome(t *testing.T) {
	t.Parallel()

	invocationID, _ := invocationdomain.ParseInvocationID("inv_00000000000000000000000000000001")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	now := time.Now().UTC()

	_, err := NewRecord(
		invocationID,
		toolID,
		&providerID,
		now,
		now,
		policydomain.DecisionAllow,
		"local.default",
		ApprovalNotRequired,
		StatusCompleted,
		invocationdomain.ResultKindTask,
		ErrorNone,
	)
	if err == nil {
		t.Fatal("NewRecord() expected inconsistent outcome error")
	}
}
