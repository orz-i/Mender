package application

import (
	"sync/atomic"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

// InvocationObserver receives best-effort, data-minimized runtime events.
type InvocationObserver interface {
	Observe(invocationdomain.Event)
}

// ProgressReporter is the restricted capability exposed to ToolExecutor.
type ProgressReporter interface {
	Report(invocationdomain.ProgressUpdate)
}

type eventEmitter struct {
	observer     InvocationObserver
	invocationID invocationdomain.InvocationID
	clock        Clock
	terminal     atomic.Bool
}

func newEventEmitter(
	observer InvocationObserver,
	invocationID invocationdomain.InvocationID,
	clock Clock,
) *eventEmitter {
	return &eventEmitter{
		observer:     observer,
		invocationID: invocationID,
		clock:        clock,
	}
}

func (e *eventEmitter) lifecycle(kind invocationdomain.EventKind) {
	if e == nil || e.observer == nil || e.terminal.Load() {
		return
	}
	event, err := invocationdomain.LifecycleEvent(e.invocationID, kind, e.clock.Now())
	if err != nil {
		return
	}
	e.observe(event)
}

func (e *eventEmitter) terminalOutcome(outcome invocationdomain.Outcome) {
	if e == nil || e.observer == nil {
		return
	}
	if !e.terminal.CompareAndSwap(false, true) {
		return
	}
	event, err := invocationdomain.TerminalEvent(e.invocationID, outcome, e.clock.Now())
	if err != nil {
		return
	}
	e.observe(event)
}

func (e *eventEmitter) Report(update invocationdomain.ProgressUpdate) {
	if e == nil || e.observer == nil || e.terminal.Load() {
		return
	}
	if err := update.Validate(); err != nil {
		return
	}
	event, err := invocationdomain.ProgressEvent(e.invocationID, update, e.clock.Now())
	if err != nil {
		return
	}
	if e.terminal.Load() {
		return
	}
	e.observe(event)
}

func (e *eventEmitter) observe(event invocationdomain.Event) {
	if e == nil || e.observer == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	e.observer.Observe(event)
}
