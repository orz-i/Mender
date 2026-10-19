package domain

// ExecutionEvent is trusted Runtime-to-storage lifecycle evidence.
// Only session creation may carry an opaque native SessionID, which never
// appears in Task/Tool JSON projections. No prompts or native tool data.
type ExecutionEventKind string

const (
	EventProcessStarted  ExecutionEventKind = "process_started"
	EventSessionReady    ExecutionEventKind = "session_ready"
	EventPromptSent      ExecutionEventKind = "prompt_sent"
	EventProgress        ExecutionEventKind = "progress"
	EventTerminalSeen    ExecutionEventKind = "terminal_seen"
	EventLeaderReaped    ExecutionEventKind = "leader_reaped"
	EventWorkerHeartbeat ExecutionEventKind = "worker_heartbeat"
)

type ExecutionEvent struct {
	Kind               ExecutionEventKind
	ProcessPID         int
	NativeSession      string
	ExistingSessionRef string // exact SessionRef when continuing, only used by trusted runner
	CanLoad            bool
	CanResume          bool
}
