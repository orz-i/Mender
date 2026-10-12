package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type TaskID string

func ParseTaskID(raw string) (TaskID, error) {
	if len(raw) != 38 || !strings.HasPrefix(raw, "atask_") {
		return "", fmt.Errorf("%w: AgentTaskID", ErrInvalid)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(raw, "atask_")); err != nil {
		return "", fmt.Errorf("%w: AgentTaskID: %w", ErrInvalid, err)
	}
	return TaskID(raw), nil
}

func NewTaskID() (TaskID, error) {
	id, err := NewID()
	if err != nil {
		return "", err
	}
	return TaskID("atask_" + strings.TrimPrefix(string(id), "agent_")), nil
}

type TaskStatus string

const (
	TaskQueued    TaskStatus = "queued"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
	TaskUnknown   TaskStatus = "unknown"
)

func (s TaskStatus) Terminal() bool {
	switch s {
	case TaskCompleted, TaskFailed, TaskCancelled, TaskUnknown:
		return true
	default:
		return false
	}
}

func (s TaskStatus) Valid() bool {
	switch s {
	case TaskQueued, TaskRunning, TaskCompleted, TaskFailed, TaskCancelled, TaskUnknown:
		return true
	default:
		return false
	}
}

// Task intentionally does not contain the input prompt; prompts stay in short-lived
// runner memory and are not copied into global Audit or task persistence.
type Task struct {
	ID              TaskID     `json:"taskId"`
	AgentID         ID         `json:"agentId"`
	Status          TaskStatus `json:"status"`
	Origin          string     `json:"-"`
	PrincipalKind   string     `json:"-"`
	PrincipalRef    string     `json:"-"`
	ScopeKind       string     `json:"-"`
	ScopeRef        string     `json:"-"`
	Generation      uint64     `json:"-"`
	Result          string     `json:"result,omitempty"`
	Diagnostic      string     `json:"diagnostic,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	CancelRequested bool       `json:"cancelRequested"`
}

const MaxResultBytes = 64 << 10
const MaxPromptBytes = 64 << 10

func (t Task) Validate() error {
	if _, err := ParseTaskID(string(t.ID)); err != nil {
		return err
	}
	if _, err := ParseID(string(t.AgentID)); err != nil {
		return err
	}
	if !t.Status.Valid() || t.Generation == 0 || t.CreatedAt.IsZero() || t.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: incomplete Task lifecycle", ErrInvalid)
	}
	if t.PrincipalKind == "" || t.PrincipalRef == "" || t.ScopeKind == "" || t.Origin == "" ||
		len(t.PrincipalRef) > 255 || len(t.ScopeRef) > 255 {
		return fmt.Errorf("%w: incomplete Task caller identity", ErrInvalid)
	}
	if len(t.Result) > MaxResultBytes || len(t.Diagnostic) > 1024 {
		return fmt.Errorf("%w: Task output exceeds limit", ErrInvalid)
	}
	return nil
}

func (t Task) OwnedBy(kind, ref, scopeKind, scopeRef string) bool {
	return t.PrincipalKind == kind && t.PrincipalRef == ref &&
		t.ScopeKind == scopeKind && t.ScopeRef == scopeRef
}

func NextStatus(old, next TaskStatus) error {
	if !old.Valid() || !next.Valid() {
		return fmt.Errorf("%w: invalid task status", ErrInvalid)
	}
	if old.Terminal() {
		return fmt.Errorf("%w: terminal task is immutable", ErrConflict)
	}
	if old == TaskQueued && (next == TaskRunning || next == TaskFailed || next == TaskCancelled || next == TaskUnknown) {
		return nil
	}
	if old == TaskRunning && next.Terminal() {
		return nil
	}
	return errors.Join(ErrConflict, fmt.Errorf("invalid transition %s → %s", old, next))
}
