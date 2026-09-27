package binding

type SystemStatus struct {
	Ready       bool             `json:"ready"`
	Policy      PolicyStatus     `json:"policy"`
	Credentials CredentialStatus `json:"credentials"`
	MCP         MCPStatus        `json:"mcp"`
}

type MCPStatus struct {
	Stdio          bool     `json:"stdio"`
	StreamableHTTP bool     `json:"streamableHttp"`
	ExposureModes  []string `json:"exposureModes"`
}

type PolicyStatus struct {
	Mode     string `json:"mode"`
	Editable bool   `json:"editable"`
	Approval string `json:"approval"`
}

type CredentialStatus struct {
	Resolver        string `json:"resolver"`
	PersistentStore bool   `json:"persistentStore"`
	Editable        bool   `json:"editable"`
}

type Provider struct {
	ProviderID  string            `json:"providerId"`
	Namespace   string            `json:"namespace"`
	DisplayName string            `json:"displayName"`
	Description string            `json:"description"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
	Transport   string            `json:"transport"`
	ToolCount   int               `json:"toolCount"`
	Endpoint    string            `json:"endpoint,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Executable  string            `json:"executable,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Directory   string            `json:"directory,omitempty"`
}

type ProviderList struct {
	Providers []Provider `json:"providers"`
}

type ProviderMutationResponse struct {
	Provider Provider   `json:"provider"`
	Sync     SyncResult `json:"sync"`
}

type SyncResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Removed int `json:"removed"`
	Total   int `json:"total"`
}

type ToolSummary struct {
	Rank        int    `json:"rank"`
	ToolID      string `json:"toolId"`
	ProviderID  string `json:"providerId"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type ToolList struct {
	Tools []ToolSummary `json:"tools"`
}

type MCPServerInstance struct {
	ServerID  string `json:"serverId"`
	Profile   string `json:"profile"`
	Transport string `json:"transport"`
	Address   string `json:"address"`
	Endpoint  string `json:"endpoint"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"`
}

type MCPServerList struct {
	Servers []MCPServerInstance `json:"servers"`
}

type ToolDetail struct {
	ToolID       string   `json:"toolId"`
	ProviderID   string   `json:"providerId"`
	SourceKey    string   `json:"sourceKey"`
	Name         string   `json:"name"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	InputSchema  any      `json:"inputSchema"`
	OutputSchema any      `json:"outputSchema,omitempty"`
	Tags         []string `json:"tags"`
}

type InvocationResult struct {
	InvocationID string `json:"invocationId"`
	Kind         string `json:"kind"`
	Content      any    `json:"content,omitempty"`
	RequestID    string `json:"requestId,omitempty"`
	Message      string `json:"message,omitempty"`
	InputSchema  any    `json:"inputSchema,omitempty"`
	TaskID       string `json:"taskId,omitempty"`
	Status       string `json:"status,omitempty"`
	Source       string `json:"source,omitempty"`
	Code         string `json:"code,omitempty"`
}

type ProfileTool struct {
	ToolID string `json:"toolId"`
	Direct bool   `json:"direct"`
}

type Profile struct {
	ProfileID   string        `json:"profileId"`
	Slug        string        `json:"slug"`
	DisplayName string        `json:"displayName"`
	Mode        string        `json:"mode"`
	Tools       []ProfileTool `json:"tools"`
}

type ProfileList struct {
	Profiles []Profile `json:"profiles"`
}

type AuditRecord struct {
	InvocationID   string `json:"invocationId"`
	ToolID         string `json:"toolId"`
	ProviderID     string `json:"providerId,omitempty"`
	Caller         string `json:"caller"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	PolicyKind     string `json:"policyKind"`
	PolicyReason   string `json:"policyReason"`
	Approval       string `json:"approval"`
	Status         string `json:"status"`
	ResultKind     string `json:"resultKind"`
	ErrorClass     string `json:"errorClass"`
	ErrorSummary   string `json:"errorSummary"`
	Origin         string `json:"origin"`
	ScopeKind      string `json:"scopeKind"`
	ScopeReference string `json:"scopeReference"`
	OutcomeKind    string `json:"outcomeKind"`
	RejectionCode  string `json:"rejectionCode"`
	FailureSource  string `json:"failureSource"`
	FailureCode    string `json:"failureCode"`
}

type AuditList struct {
	Records []AuditRecord `json:"records"`
}
