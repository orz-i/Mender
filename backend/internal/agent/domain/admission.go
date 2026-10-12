package domain

import "time"

// AdmissionEvent is data-minimized governance evidence for one Agent action.
// Attribution references are persisted but never serialized to read-only UI,
// CLI, SDK, MCP, or downstream Agent content.
type AdmissionEvent struct {
	AgentID       ID        `json:"agentId"`
	Action        string    `json:"action"`
	PrincipalKind string    `json:"principalKind"`
	PrincipalRef  string    `json:"-"`
	Origin        string    `json:"origin"`
	ScopeKind     string    `json:"scopeKind"`
	ScopeRef      string    `json:"-"`
	PolicyKind    string    `json:"policyKind"`
	PolicyReason  string    `json:"policyReason"`
	Status        string    `json:"status"`
	OccurredAt    time.Time `json:"occurredAt"`
}
