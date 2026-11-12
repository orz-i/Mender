package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type TaskEventID string

func NewTaskEventID() (TaskEventID, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return TaskEventID("aevt_" + hex.EncodeToString(entropy[:])), nil
}

func ParseTaskEventID(input string) (TaskEventID, error) {
	if len(input) != 37 || !strings.HasPrefix(input, "aevt_") {
		return "", fmt.Errorf("%w: TaskEventID", ErrInvalid)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(input, "aevt_")); err != nil {
		return "", fmt.Errorf("%w: TaskEventID: %w", ErrInvalid, err)
	}
	return TaskEventID(input), nil
}

type TaskEventType string

const (
	TaskEventUserPrompt   TaskEventType = "user_prompt"
	TaskEventThinking     TaskEventType = "thinking"
	TaskEventMessageChunk TaskEventType = "message_chunk"
	TaskEventToolCall     TaskEventType = "tool_call"
	TaskEventToolResult   TaskEventType = "tool_result"
	TaskEventApproval     TaskEventType = "approval"
	TaskEventTerminal     TaskEventType = "terminal"
)

func (t TaskEventType) Valid() bool {
	switch t {
	case TaskEventUserPrompt,
		TaskEventThinking,
		TaskEventMessageChunk,
		TaskEventToolCall,
		TaskEventToolResult,
		TaskEventApproval,
		TaskEventTerminal:
		return true
	default:
		return false
	}
}

type TaskEvent struct {
	ID        TaskEventID    `json:"eventId"`
	TaskID    TaskID         `json:"taskId"`
	Seq       int64          `json:"seq"`
	Type      TaskEventType  `json:"type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"createdAt"`
}

func (e TaskEvent) Validate() error {
	if _, err := ParseTaskEventID(string(e.ID)); err != nil {
		return err
	}
	if _, err := ParseTaskID(string(e.TaskID)); err != nil {
		return err
	}
	if e.Seq <= 0 {
		return fmt.Errorf("%w: invalid event seq", ErrInvalid)
	}
	if !e.Type.Valid() {
		return fmt.Errorf("%w: invalid event type", ErrInvalid)
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("%w: missing event timestamp", ErrInvalid)
	}
	return nil
}
