package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestRecordAndListTaskEvents(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.sqlite")
	s, err := openAgentV3(t, ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial listing should be empty
	events, err := s.ListTaskEvents(ctx, taskID)
	if err != nil {
		t.Fatalf("failed to list empty events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}

	// 2. Record user_prompt event with explicit seq 1
	evt1ID, _ := agentdomain.NewTaskEventID()
	evt1 := agentdomain.TaskEvent{
		ID:        evt1ID,
		TaskID:    taskID,
		Seq:       1,
		Type:      agentdomain.TaskEventUserPrompt,
		Payload:   map[string]any{"prompt": "Fix bug in auth"},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.RecordTaskEvent(ctx, evt1); err != nil {
		t.Fatalf("failed to record evt1: %v", err)
	}

	// 3. Record thinking event with auto seq (seq 0)
	evt2 := agentdomain.TaskEvent{
		TaskID:    taskID,
		Type:      agentdomain.TaskEventThinking,
		Payload:   map[string]any{"thought": "Analyzing codebase"},
		CreatedAt: time.Now().UTC().Add(time.Second),
	}
	if err := s.RecordTaskEvent(ctx, evt2); err != nil {
		t.Fatalf("failed to record evt2 with auto seq: %v", err)
	}

	// 4. Record tool_call event with explicit seq 3
	evt3ID, _ := agentdomain.NewTaskEventID()
	evt3 := agentdomain.TaskEvent{
		ID:        evt3ID,
		TaskID:    taskID,
		Seq:       3,
		Type:      agentdomain.TaskEventToolCall,
		Payload:   map[string]any{"tool": "read_file", "path": "auth.go"},
		CreatedAt: time.Now().UTC().Add(2 * time.Second),
	}
	if err := s.RecordTaskEvent(ctx, evt3); err != nil {
		t.Fatalf("failed to record evt3: %v", err)
	}

	// 5. Record terminal event with auto seq (should become 4)
	evt4 := agentdomain.TaskEvent{
		TaskID:    taskID,
		Type:      agentdomain.TaskEventTerminal,
		Payload:   map[string]any{"status": "completed", "result": "All good"},
		CreatedAt: time.Now().UTC().Add(3 * time.Second),
	}
	if err := s.RecordTaskEvent(ctx, evt4); err != nil {
		t.Fatalf("failed to record evt4: %v", err)
	}

	// 6. List and verify all 4 events
	all, err := s.ListTaskEvents(ctx, taskID)
	if err != nil {
		t.Fatalf("failed to list events: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 events, got %d", len(all))
	}

	if all[0].Seq != 1 || all[0].Type != agentdomain.TaskEventUserPrompt {
		t.Errorf("expected event 0 to be user_prompt seq 1, got %+v", all[0])
	}
	if all[0].Payload["prompt"] != "Fix bug in auth" {
		t.Errorf("expected prompt in payload, got %+v", all[0].Payload)
	}

	if all[1].Seq != 2 || all[1].Type != agentdomain.TaskEventThinking {
		t.Errorf("expected event 1 to be thinking seq 2, got %+v", all[1])
	}

	if all[2].Seq != 3 || all[2].Type != agentdomain.TaskEventToolCall {
		t.Errorf("expected event 2 to be tool_call seq 3, got %+v", all[2])
	}
	if all[2].Payload["tool"] != "read_file" {
		t.Errorf("expected tool payload, got %+v", all[2].Payload)
	}

	if all[3].Seq != 4 || all[3].Type != agentdomain.TaskEventTerminal {
		t.Errorf("expected event 3 to be terminal seq 4, got %+v", all[3])
	}

	// 7. Verify duplicate seq fails
	dupEvtID, _ := agentdomain.NewTaskEventID()
	dup := agentdomain.TaskEvent{
		ID:        dupEvtID,
		TaskID:    taskID,
		Seq:       1,
		Type:      agentdomain.TaskEventUserPrompt,
		Payload:   map[string]any{"prompt": "dup"},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.RecordTaskEvent(ctx, dup); err == nil {
		t.Fatal("expected error on duplicate seq, got nil")
	}

	// 8. Verify invalid task ID in ListTaskEvents
	if _, err := s.ListTaskEvents(ctx, agentdomain.TaskID("invalid")); err == nil {
		t.Fatal("expected error for invalid task ID")
	}
}
