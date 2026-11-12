package domain

import (
	"testing"
	"time"
)

func TestTaskEventValidation(t *testing.T) {
	taskID, err := NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	eventID, err := NewTaskEventID()
	if err != nil {
		t.Fatal(err)
	}

	evt := TaskEvent{
		ID:        eventID,
		TaskID:    taskID,
		Seq:       1,
		Type:      TaskEventUserPrompt,
		Payload:   map[string]any{"prompt": "hello"},
		CreatedAt: time.Now().UTC(),
	}
	if err := evt.Validate(); err != nil {
		t.Fatalf("expected valid task event, got %v", err)
	}

	// Invalid ID
	badID := evt
	badID.ID = "bad_id"
	if err := badID.Validate(); err == nil {
		t.Fatal("expected error for bad ID")
	}

	// Invalid TaskID
	badTaskID := evt
	badTaskID.TaskID = "bad_task"
	if err := badTaskID.Validate(); err == nil {
		t.Fatal("expected error for bad task ID")
	}

	// Invalid Seq
	badSeq := evt
	badSeq.Seq = 0
	if err := badSeq.Validate(); err == nil {
		t.Fatal("expected error for zero seq")
	}

	// Invalid Type
	badType := evt
	badType.Type = "unknown_type"
	if err := badType.Validate(); err == nil {
		t.Fatal("expected error for unknown type")
	}

	// Zero CreatedAt
	badTime := evt
	badTime.CreatedAt = time.Time{}
	if err := badTime.Validate(); err == nil {
		t.Fatal("expected error for zero CreatedAt")
	}
}

func TestTaskEventTypeValidity(t *testing.T) {
	for _, valid := range []TaskEventType{
		TaskEventUserPrompt,
		TaskEventThinking,
		TaskEventMessageChunk,
		TaskEventToolCall,
		TaskEventToolResult,
		TaskEventApproval,
		TaskEventTerminal,
	} {
		if !valid.Valid() {
			t.Fatalf("expected %s to be valid", valid)
		}
	}

	if TaskEventType("bogus").Valid() {
		t.Fatal("bogus should not be valid")
	}
}
