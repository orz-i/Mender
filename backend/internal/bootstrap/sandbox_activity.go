package bootstrap

import (
	"context"
	"fmt"
	"sort"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const maxSandboxActivityLimit = 100

// SandboxActivityEvent is a deliberate *non-secret* operator-facing projection.
// Principal references, policy reasons, credentials, remote endpoints, tool
// arguments and provider error bodies MUST NOT be added to this response.
type SandboxActivityEvent struct {
	Kind           string    `json:"kind"`
	Action         string    `json:"action"`
	Status         string    `json:"status"`
	OccurredAt     time.Time `json:"occurredAt"`
	PrincipalKind  string    `json:"principalKind"`
	Origin         string    `json:"origin"`
	PolicyDecision string    `json:"policyDecision,omitempty"`
	Generation     uint64    `json:"generation,omitempty"`
	PreviousState  string    `json:"previousState,omitempty"`
	NextState      string    `json:"nextState,omitempty"`
}
type SandboxActivity struct {
	SandboxID string                 `json:"sandboxId"`
	Events    []SandboxActivityEvent `json:"events"`
	Limit     int                    `json:"limit"`
}

// Activity merges two durable, independently audited event streams. Both
// queries are bounded and read-only; no Backend, credentials or remote service
// is contacted. Admission 'denied' events remain visible alongside successes.
func (s *Sandboxes) Activity(ctx context.Context, id sandboxdomain.ID, limit int) (SandboxActivity, error) {
	if s == nil || s.store == nil {
		return SandboxActivity{}, sandboxdomain.ErrBackendUnavailable
	}
	if limit < 1 || limit > maxSandboxActivityLimit {
		return SandboxActivity{}, fmt.Errorf("sandbox activity limit must be between 1 and %d", maxSandboxActivityLimit)
	}
	if _, err := s.store.Get(ctx, id); err != nil {
		return SandboxActivity{}, err
	}
	admissions, err := s.store.ListSandboxAdmissions(ctx, id, limit)
	if err != nil {
		return SandboxActivity{}, fmt.Errorf("sandbox activity: admissions: %w", err)
	}
	lifecycle, err := s.store.ListLifecycleEvents(ctx, id, limit)
	if err != nil {
		return SandboxActivity{}, fmt.Errorf("sandbox activity: lifecycle: %w", err)
	}
	events := make([]SandboxActivityEvent, 0, len(admissions)+len(lifecycle))
	for _, v := range admissions {
		events = append(events, SandboxActivityEvent{
			Kind: "admission", Action: string(v.Action), Status: v.Status,
			OccurredAt: v.FinishedAt.UTC(), PrincipalKind: string(v.PrincipalKind), Origin: string(v.Origin),
			PolicyDecision: string(v.PolicyKind),
		})
	}
	for _, v := range lifecycle {
		events = append(events, SandboxActivityEvent{
			Kind: "lifecycle", Action: string(v.Action), Status: "completed",
			OccurredAt: v.FinishedAt.UTC(), PrincipalKind: string(v.PrincipalKind), Origin: string(v.Origin),
			Generation: v.Generation, PreviousState: string(v.Previous), NextState: string(v.Next),
		})
	}
	// Sorting on native UTC time, not variable-precision timestamp strings.
	// Stable ordering makes same-time admission/transition events predictable.
	sort.SliceStable(events, func(i, j int) bool { return events[i].OccurredAt.After(events[j].OccurredAt) })
	if len(events) > limit {
		events = events[:limit]
	}
	return SandboxActivity{SandboxID: id.String(), Events: events, Limit: limit}, nil
}
