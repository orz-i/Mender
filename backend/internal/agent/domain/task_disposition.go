package domain

import "time"

// UnknownTaskDisposition is an owner-reported work-handling annotation, not
// evidence that the original unknown AgentTask completed. No Agent mutation
// is authorized by this record.
type UnknownTaskDisposition struct {
	Handling   string    `json:"handling"`
	RecordedAt time.Time `json:"recordedAt"`
}

// ResumeAvailability is a read-only owner's preview. Only Resume admission
// may decide whether a new Task can be created; preview alone is not a grant.
type ResumeAvailability struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason"`
}

const (
	UnknownHandledElsewhere = "handled_elsewhere"
	UnknownNeedsReview      = "needs_review"
)
