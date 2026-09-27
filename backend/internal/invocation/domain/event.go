package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const maxProgressMessageRunes = 512

// EventKind identifies one ephemeral Invocation lifecycle signal.
type EventKind string

const (
	EventAccepted           EventKind = "accepted"
	EventScopeResolved      EventKind = "scope_resolved"
	EventArgumentsValidated EventKind = "arguments_validated"
	EventPolicyDecided      EventKind = "policy_decided"
	EventApprovalResolved   EventKind = "approval_resolved"
	EventExecutionWaiting   EventKind = "execution_waiting"
	EventExecutionStarted   EventKind = "execution_started"
	EventProgress           EventKind = "progress"
	EventTerminal           EventKind = "terminal"
)

// ProgressUpdate is a bounded executor-to-Invocation progress message.
type ProgressUpdate struct {
	message string
}

func NewProgressUpdate(message string) (ProgressUpdate, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return ProgressUpdate{}, validationError("progress.message", "must not be empty")
	}
	if utf8.RuneCountInString(message) > maxProgressMessageRunes {
		return ProgressUpdate{}, validationError("progress.message", "is too long")
	}
	return ProgressUpdate{message: message}, nil
}

func (u ProgressUpdate) Validate() error {
	_, err := NewProgressUpdate(u.message)
	return err
}

func (u ProgressUpdate) Message() string {
	return u.message
}

// Event is a data-minimized ephemeral Invocation lifecycle signal.
type Event struct {
	invocationID  InvocationID
	kind          EventKind
	occurredAt    time.Time
	message       string
	outcomeKind   OutcomeKind
	rejectionCode RejectionCode
	failureSource FailureSource
	failureCode   FailureCode
}

func LifecycleEvent(
	invocationID InvocationID,
	kind EventKind,
	occurredAt time.Time,
) (Event, error) {
	if _, err := ParseInvocationID(invocationID.String()); err != nil {
		return Event{}, err
	}
	if !validLifecycleEventKind(kind) {
		return Event{}, validationError("event.kind", fmt.Sprintf("invalid lifecycle kind %q", kind))
	}
	if occurredAt.IsZero() {
		return Event{}, validationError("event.occurred_at", "must not be zero")
	}
	return Event{
		invocationID: invocationID,
		kind:         kind,
		occurredAt:   occurredAt.UTC(),
	}, nil
}

func ProgressEvent(
	invocationID InvocationID,
	update ProgressUpdate,
	occurredAt time.Time,
) (Event, error) {
	if err := update.Validate(); err != nil {
		return Event{}, err
	}
	event, err := baseSpecialEvent(invocationID, EventProgress, occurredAt)
	if err != nil {
		return Event{}, err
	}
	event.message = update.Message()
	return event, nil
}

func TerminalEvent(
	invocationID InvocationID,
	outcome Outcome,
	occurredAt time.Time,
) (Event, error) {
	if outcome.Kind() == "" {
		return Event{}, validationError("event.outcome_kind", "must not be empty")
	}
	event, err := baseSpecialEvent(invocationID, EventTerminal, occurredAt)
	if err != nil {
		return Event{}, err
	}
	event.outcomeKind = outcome.Kind()
	event.rejectionCode = outcome.RejectionCode()
	event.failureSource = outcome.FailureSource()
	event.failureCode = outcome.FailureCode()
	return event, nil
}

func baseSpecialEvent(
	invocationID InvocationID,
	kind EventKind,
	occurredAt time.Time,
) (Event, error) {
	if _, err := ParseInvocationID(invocationID.String()); err != nil {
		return Event{}, err
	}
	if occurredAt.IsZero() {
		return Event{}, validationError("event.occurred_at", "must not be zero")
	}
	return Event{
		invocationID: invocationID,
		kind:         kind,
		occurredAt:   occurredAt.UTC(),
	}, nil
}

func validLifecycleEventKind(kind EventKind) bool {
	switch kind {
	case EventAccepted,
		EventScopeResolved,
		EventArgumentsValidated,
		EventPolicyDecided,
		EventApprovalResolved,
		EventExecutionWaiting,
		EventExecutionStarted:
		return true
	default:
		return false
	}
}

func (e Event) InvocationID() InvocationID {
	return e.invocationID
}

func (e Event) Kind() EventKind {
	return e.kind
}

func (e Event) OccurredAt() time.Time {
	return e.occurredAt
}

func (e Event) Message() string {
	return e.message
}

func (e Event) OutcomeKind() OutcomeKind {
	return e.outcomeKind
}

func (e Event) RejectionCode() RejectionCode {
	return e.rejectionCode
}

func (e Event) FailureSource() FailureSource {
	return e.failureSource
}

func (e Event) FailureCode() FailureCode {
	return e.failureCode
}
