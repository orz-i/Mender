package domain

import "time"

// TaskObservation is a bounded, owner-safe execution projection. It never
// contains native SessionIDs, process IDs, workspace roots, or raw Agent events.
// Activity is local_active only when the current Runtime owns the worker;
// other runtimes cannot infer liveness from a persisted PID or timestamp.
type TaskObservation struct {
	Phase                 string     `json:"phase"`
	Activity              string     `json:"activity"`
	LastObservedAt        time.Time  `json:"lastObservedAt"`
	LastNativeProgressAt  *time.Time `json:"lastNativeProgressAt,omitempty"` // native Agent progress, never worker heartbeat
	LastWorkerHeartbeatAt *time.Time `json:"lastWorkerHeartbeatAt,omitempty"`
	SessionRecorded       bool       `json:"sessionRecorded"`
	SessionRecoverability string     `json:"sessionRecoverability"`
	LeaderReaped          bool       `json:"leaderReaped,omitempty"`
}
