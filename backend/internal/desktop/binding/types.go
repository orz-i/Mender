package binding

type SystemStatus struct {
	Ready       bool             `json:"ready"`
	Policy      PolicyStatus     `json:"policy"`
	Credentials CredentialStatus `json:"credentials"`
	MCP         MCPStatus        `json:"mcp"`
}

// Sandbox is management metadata; state reflects the last persisted observation.
type Sandbox struct {
	SandboxID        string `json:"sandboxId"`
	Name             string `json:"name"`
	Backend          string `json:"backend"`
	Isolation        string `json:"isolation"`
	State            string `json:"state"`
	LocalRoot        string `json:"localRoot"`
	OwnerKind        string `json:"ownerKind"`
	OwnerRef         string `json:"ownerRef"`
	ActiveLeases     int    `json:"activeLeases"`
	BackendAvailable bool   `json:"backendAvailable"`
}

// Activity projection omits subject references, policy reasons and secrets.
type SandboxActivityEvent struct {
	Kind           string `json:"kind"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	OccurredAt     string `json:"occurredAt"`
	PrincipalKind  string `json:"principalKind"`
	Origin         string `json:"origin"`
	PolicyDecision string `json:"policyDecision,omitempty"`
	Generation     uint64 `json:"generation,omitempty"`
	PreviousState  string `json:"previousState,omitempty"`
	NextState      string `json:"nextState,omitempty"`
}
type SandboxActivity struct {
	SandboxID string                 `json:"sandboxId"`
	Events    []SandboxActivityEvent `json:"events"`
	Limit     int                    `json:"limit"`
}
type SandboxList struct {
	Sandboxes []Sandbox `json:"sandboxes"`
}
type DockerBackendStatus struct {
	DaytonaConfigured bool   `json:"daytonaConfigured"`
	DaytonaStatus     string `json:"daytonaStatus"`
	DockerAvailable   bool   `json:"dockerAvailable"`
	DockerStatus      string `json:"dockerStatus"`
}
type CreateDaytonaSandboxInput struct {
	Name          string `json:"name"`
	CredentialRef string `json:"credentialRef"`
	ImageDigest   string `json:"imageDigest"`
	Target        string `json:"target"`
	CPU           int    `json:"cpu"`
	MemoryGiB     int    `json:"memoryGiB"`
	DiskGiB       int    `json:"diskGiB"`
	TTLMinutes    int    `json:"ttlMinutes"`
}
type CreateDockerSandboxInput struct {
	Name         string `json:"name"`
	ImageDigest  string `json:"imageDigest"`
	UserID       uint32 `json:"userId"`
	GroupID      uint32 `json:"groupId"`
	CPUmilli     uint32 `json:"cpuMilli"`
	MemoryMiB    uint32 `json:"memoryMiB"`
	WorkspaceMiB uint32 `json:"workspaceMiB"`
	PIDs         uint32 `json:"pids"`
}
type CreateLocalSandboxInput struct {
	Name      string `json:"name"`
	Root      string `json:"root"`
	Isolation string `json:"isolation"`
}

type RuntimeBudget struct {
	MaxInvocations          int   `json:"maxInvocations"`
	MaxPrincipalInvocations int   `json:"maxPrincipalInvocations"`
	MaxProviderInvocations  int   `json:"maxProviderInvocations"`
	MaxToolInvocations      int   `json:"maxToolInvocations"`
	MaxProviderActivations  int   `json:"maxProviderActivations"`
	MaxExecutionTimeoutMS   int64 `json:"maxExecutionTimeoutMs"`
	MaxArgumentBytes        int   `json:"maxArgumentBytes"`
	MaxResultBytes          int   `json:"maxResultBytes"`
}

type RuntimeIsolation struct {
	WorkspaceShell   string `json:"workspaceShell"`
	ExternalProvider string `json:"externalProvider"`
	TunnelClient     string `json:"tunnelClient"`
}

type RuntimeBackend struct {
	Name                    string `json:"name"`
	RestrictedHostAvailable bool   `json:"restrictedHostAvailable"`
}

type RuntimeUsage struct {
	Invocations int `json:"invocations"`
	Activations int `json:"activations"`
}

type RuntimeGovernanceStatus struct {
	Budget    RuntimeBudget    `json:"budget"`
	Isolation RuntimeIsolation `json:"isolation"`
	Backend   RuntimeBackend   `json:"backend"`
	Usage     RuntimeUsage     `json:"usage"`
}

type MCPStatus struct {
	Stdio          bool     `json:"stdio"`
	StreamableHTTP bool     `json:"streamableHttp"`
	ExposureModes  []string `json:"exposureModes"`
}

type PolicyStatus struct {
	Mode             string `json:"mode"`
	Editable         bool   `json:"editable"`
	Approval         string `json:"approval"`
	DefaultDecision  string `json:"defaultDecision"`
	RuleCount        int    `json:"ruleCount"`
	EnabledRuleCount int    `json:"enabledRuleCount"`
}

type Policy struct {
	DefaultDecision string       `json:"defaultDecision"`
	Rules           []PolicyRule `json:"rules"`
}

type PolicyRule struct {
	RuleID      string      `json:"ruleId"`
	DisplayName string      `json:"displayName"`
	Enabled     bool        `json:"enabled"`
	Decision    string      `json:"decision"`
	Match       PolicyMatch `json:"match"`
	CreatedAt   string      `json:"createdAt"`
	UpdatedAt   string      `json:"updatedAt"`
}

type PolicyRuleInput struct {
	DisplayName string      `json:"displayName"`
	Enabled     bool        `json:"enabled"`
	Decision    string      `json:"decision"`
	Match       PolicyMatch `json:"match"`
}

type PolicyMatch struct {
	Actions        []string                `json:"actions"`
	Origins        []string                `json:"origins"`
	PrincipalKinds []string                `json:"principalKinds"`
	PrincipalRefs  []string                `json:"principalRefs"`
	ScopeKinds     []string                `json:"scopeKinds"`
	ProfileRefs    []string                `json:"profileRefs"`
	ProviderIDs    []string                `json:"providerIds"`
	ToolIDs        []string                `json:"toolIds"`
	FactKnowledge  string                  `json:"factKnowledge"`
	EffectsAny     []string                `json:"effectsAny"`
	ResourcesAny   []PolicyResourceMatcher `json:"resourcesAny"`
}

type PolicyResourceMatcher struct {
	Kind     string `json:"kind"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type CredentialStatus struct {
	Resolver        string `json:"resolver"`
	PersistentStore bool   `json:"persistentStore"`
	Editable        bool   `json:"editable"`
}

type Credential struct {
	Reference           string `json:"reference"`
	Namespace           string `json:"namespace"`
	Name                string `json:"name"`
	DisplayName         string `json:"displayName"`
	Source              string `json:"source"`
	EnvironmentVariable string `json:"environmentVariable"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

type CredentialList struct {
	Credentials []Credential `json:"credentials"`
}

type CredentialHeader struct {
	Name          string `json:"name"`
	Scheme        string `json:"scheme"`
	CredentialRef string `json:"credentialRef"`
}

type CredentialEnvironment struct {
	Name          string `json:"name"`
	CredentialRef string `json:"credentialRef"`
}

type ProviderPlugin struct {
	PluginID        string   `json:"pluginId"`
	Version         string   `json:"version"`
	PackageVersion  string   `json:"packageVersion"`
	DisplayName     string   `json:"displayName"`
	Description     string   `json:"description"`
	ConfigVersion   int      `json:"configVersion"`
	SourceKind      string   `json:"sourceKind"`
	InstallationID  string   `json:"installationId,omitempty"`
	ProtocolVersion uint32   `json:"protocolVersion,omitempty"`
	ConfigSchema    any      `json:"configSchema,omitempty"`
	UISchema        any      `json:"uiSchema,omitempty"`
	Capabilities    []string `json:"capabilities"`
	Enabled         bool     `json:"enabled"`
}

type ProviderPluginIssue struct {
	InstallationID string `json:"installationId"`
	Code           string `json:"code"`
	Message        string `json:"message"`
}

type ProviderPluginList struct {
	Plugins []ProviderPlugin      `json:"plugins"`
	Issues  []ProviderPluginIssue `json:"issues"`
}

type ProviderPluginInstallation struct {
	InstallationID  string   `json:"installationId"`
	PluginID        string   `json:"pluginId"`
	PackageVersion  string   `json:"packageVersion"`
	SourceKind      string   `json:"sourceKind"`
	Digest          string   `json:"digest"`
	Status          string   `json:"status"`
	InstalledAt     string   `json:"installedAt"`
	DisplayName     string   `json:"displayName,omitempty"`
	Description     string   `json:"description,omitempty"`
	ConfigVersion   int      `json:"configVersion,omitempty"`
	ProtocolVersion uint32   `json:"protocolVersion,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
}

type ProviderPluginInstallationList struct {
	Installations []ProviderPluginInstallation `json:"installations"`
}

type SkillInstallation struct {
	SkillID         string `json:"skillId"`
	InstallationID  string `json:"installationId"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	SourceKind      string `json:"sourceKind"`
	SourceReference string `json:"sourceReference"`
	Digest          string `json:"digest"`
	VersionHint     string `json:"versionHint"`
	Trusted         bool   `json:"trusted"`
	Enabled         bool   `json:"enabled"`
	BuiltIn         bool   `json:"builtIn"`
	InstalledAt     string `json:"installedAt,omitempty"`
}

type SkillInstallationList struct {
	Installations []SkillInstallation `json:"installations"`
}

type SkillMetadata struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata"`
	AllowedTools  string            `json:"allowedTools,omitempty"`
}

type SkillResource struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Executable bool   `json:"executable"`
}

type SkillInspection struct {
	Installation SkillInstallation `json:"installation"`
	Metadata     SkillMetadata     `json:"metadata"`
	Resources    []SkillResource   `json:"resources"`
	TotalBytes   int64             `json:"totalBytes"`
}

type SkillDeployment struct {
	DeploymentID   string `json:"deploymentId"`
	InstallationID string `json:"installationId"`
	Target         string `json:"target"`
	Root           string `json:"root"`
	Destination    string `json:"destination"`
	Digest         string `json:"digest"`
	DeployedAt     string `json:"deployedAt"`
	Drift          string `json:"drift"`
	CurrentDigest  string `json:"currentDigest,omitempty"`
}

type SkillDeploymentList struct {
	Deployments []SkillDeployment `json:"deployments"`
}

type Provider struct {
	ProviderID            string                  `json:"providerId"`
	Namespace             string                  `json:"namespace"`
	DisplayName           string                  `json:"displayName"`
	Description           string                  `json:"description"`
	CreatedAt             string                  `json:"createdAt"`
	UpdatedAt             string                  `json:"updatedAt"`
	PluginID              string                  `json:"pluginId,omitempty"`
	InstallationID        string                  `json:"installationId,omitempty"`
	PluginVersion         string                  `json:"pluginVersion,omitempty"`
	PluginDisplayName     string                  `json:"pluginDisplayName,omitempty"`
	ConfigVersion         int                     `json:"configVersion,omitempty"`
	ConnectionSummary     string                  `json:"connectionSummary,omitempty"`
	Config                map[string]any          `json:"config,omitempty"`
	Transport             string                  `json:"transport"`
	ToolCount             int                     `json:"toolCount"`
	Endpoint              string                  `json:"endpoint,omitempty"`
	Headers               map[string]string       `json:"headers,omitempty"`
	Executable            string                  `json:"executable,omitempty"`
	Args                  []string                `json:"args,omitempty"`
	Directory             string                  `json:"directory,omitempty"`
	CredentialHeaders     []CredentialHeader      `json:"credentialHeaders,omitempty"`
	CredentialEnvironment []CredentialEnvironment `json:"credentialEnvironment,omitempty"`
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
	ServerID          string `json:"serverId"`
	Profile           string `json:"profile"`
	Transport         string `json:"transport"`
	Address           string `json:"address"`
	Endpoint          string `json:"endpoint"`
	Status            string `json:"status"`
	StartedAt         string `json:"startedAt"`
	TunnelID          string `json:"tunnelId,omitempty"`
	TunnelDisplayName string `json:"tunnelDisplayName,omitempty"`
	TunnelProvider    string `json:"tunnelProvider,omitempty"`
	TunnelExternalID  string `json:"tunnelExternalId,omitempty"`
	TunnelStatus      string `json:"tunnelStatus,omitempty"`
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
	ToolID                string   `json:"toolId"`
	Direct                bool     `json:"direct"`
	Alias                 string   `json:"alias"`
	TitleOverride         string   `json:"titleOverride"`
	DescriptionOverride   string   `json:"descriptionOverride"`
	PolicyConstraint      string   `json:"policyConstraint"`
	ConcurrencyConstraint string   `json:"concurrencyConstraint"`
	CredentialRefs        []string `json:"credentialRefs"`
}

type Profile struct {
	ProfileID   string        `json:"profileId"`
	Slug        string        `json:"slug"`
	DisplayName string        `json:"displayName"`
	Mode        string        `json:"mode"`
	TunnelID    string        `json:"tunnelId,omitempty"`
	Tools       []ProfileTool `json:"tools"`
}

type Tunnel struct {
	TunnelID             string `json:"tunnelId"`
	DisplayName          string `json:"displayName"`
	Provider             string `json:"provider"`
	ExternalTunnelID     string `json:"externalTunnelId"`
	RuntimeCredentialRef string `json:"runtimeCredentialRef"`
}

type TunnelList struct {
	Tunnels []Tunnel `json:"tunnels"`
}

type ProfileList struct {
	Profiles []Profile `json:"profiles"`
}

type AuditRecord struct {
	InvocationID   string `json:"invocationId"`
	ToolID         string `json:"toolId"`
	ProviderID     string `json:"providerId,omitempty"`
	PrincipalKind  string `json:"principalKind"`
	PrincipalRef   string `json:"principalRef"`
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

type ProviderActivationAuditRecord struct {
	ActivationID   string `json:"activationId"`
	ProviderID     string `json:"providerId"`
	Action         string `json:"action"`
	PrincipalKind  string `json:"principalKind"`
	PrincipalRef   string `json:"principalRef"`
	Origin         string `json:"origin"`
	ScopeKind      string `json:"scopeKind"`
	ScopeReference string `json:"scopeReference"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	PolicyKind     string `json:"policyKind"`
	PolicyReason   string `json:"policyReason"`
	Approval       string `json:"approval"`
	Status         string `json:"status"`
	ErrorClass     string `json:"errorClass"`
	ErrorSummary   string `json:"errorSummary"`
}

type ProviderActivationAuditList struct {
	Records []ProviderActivationAuditRecord `json:"records"`
}
