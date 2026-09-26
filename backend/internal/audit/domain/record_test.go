package domain

import (
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
		providerID,
		"cli",
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

func TestRecordRejectsInconsistentOutcome(t *testing.T) {
	t.Parallel()

	invocationID, _ := invocationdomain.ParseInvocationID("inv_00000000000000000000000000000001")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	now := time.Now().UTC()

	_, err := NewRecord(
		invocationID,
		toolID,
		providerID,
		"",
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
