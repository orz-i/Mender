package domain

// SessionContinuation is a trusted, backend-neutral snapshot resolved by
// Mender storage. Never expose native IDs or its fields to Profile tools.
type SessionContinuation struct {
	SourceTaskID    TaskID
	SessionRef      string
	NativeSessionID string
	AgentID         ID
	Backend         Backend
	WorkspaceRoot   string
	Isolation       string
	PrincipalKind   string
	PrincipalRef    string
	ScopeKind       string
	ScopeRef        string
	CanLoad         bool
	CanResume       bool
}

func (s SessionContinuation) Valid() bool {
	_, taskErr := ParseTaskID(string(s.SourceTaskID))
	_, agentErr := ParseID(string(s.AgentID))
	return taskErr == nil && agentErr == nil &&
		len(s.SessionRef) == 38 && len(s.NativeSessionID) > 0 &&
		len(s.NativeSessionID) <= 512 && s.WorkspaceRoot != "" &&
		(s.CanLoad || s.CanResume)
}
