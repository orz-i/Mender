package domain

import (
	"strings"
	"testing"
	"time"
)

func TestProgressUpdateIsBounded(t *testing.T) {
	t.Parallel()

	update, err := NewProgressUpdate("  indexing files  ")
	if err != nil {
		t.Fatal(err)
	}
	if update.Message() != "indexing files" {
		t.Fatalf("Message() = %q", update.Message())
	}

	if _, err := NewProgressUpdate(""); err == nil {
		t.Fatal("NewProgressUpdate() accepted empty message")
	}
	if _, err := NewProgressUpdate(strings.Repeat("x", maxProgressMessageRunes+1)); err == nil {
		t.Fatal("NewProgressUpdate() accepted oversized message")
	}
}

func TestTerminalEventCarriesOnlyOutcomeClassification(t *testing.T) {
	t.Parallel()

	id, _ := ParseInvocationID("inv_00000000000000000000000000000001")
	outcome, err := Failed(
		FailureSourceResultGuard,
		FailureOutputSchemaMismatch,
		"sensitive result content is not stored in the event",
	)
	if err != nil {
		t.Fatal(err)
	}

	event, err := TerminalEvent(id, outcome, time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind() != EventTerminal ||
		event.OutcomeKind() != OutcomeKindFailed ||
		event.FailureSource() != FailureSourceResultGuard ||
		event.FailureCode() != FailureOutputSchemaMismatch {
		t.Fatalf("terminal event = %#v", event)
	}
	if event.Message() != "" {
		t.Fatalf("terminal event retained outcome message %q", event.Message())
	}
}

func TestLifecycleEventRejectsProgressAndTerminalKinds(t *testing.T) {
	t.Parallel()

	id, _ := ParseInvocationID("inv_00000000000000000000000000000001")
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

	for _, kind := range []EventKind{EventProgress, EventTerminal, EventKind("unknown")} {
		if _, err := LifecycleEvent(id, kind, now); err == nil {
			t.Fatalf("LifecycleEvent(%q) expected error", kind)
		}
	}
}
