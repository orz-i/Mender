import type { components } from "../generated/schema";

export type PluginCatalogueEntry=components["schemas"]["PluginCatalogueEntry"];
export type IntegrationOffering=components["schemas"]["IntegrationOffering"];
export type CreateExternalSandboxRequest=components["schemas"]["CreateExternalSandboxRequest"];
export type CreateExternalTunnelRequest=components["schemas"]["CreateExternalTunnelRequest"];
export type ProviderPlugin = components["schemas"]["ProviderPlugin"];
export type ProviderPluginIssue = components["schemas"]["ProviderPluginIssue"];
export type ProviderPluginInstallation =
  components["schemas"]["ProviderPluginInstallation"];
export type SkillInstallation = components["schemas"]["SkillInstallation"];
export type SkillInspection = components["schemas"]["SkillInspection"];
export type SkillDeployment = components["schemas"]["SkillDeployment"];
export type Provider = components["schemas"]["Provider"];
export type SyncResult = components["schemas"]["SyncResult"];
export type ToolSummary = components["schemas"]["ToolSummary"];
export type ToolDetail = components["schemas"]["ToolDetail"];
export type InvocationResult = components["schemas"]["InvocationResult"];
export type Profile = components["schemas"]["Profile"];
export type Tunnel = components["schemas"]["Tunnel"];
export type Agent = components["schemas"]["Agent"];
export type AgentTools = components["schemas"]["AgentTools"];
export type AgentWorkspace = components["schemas"]["AgentWorkspace"];
export type AgentModelChoices = components["schemas"]["AgentModelChoices"];
export type AgentTask = components["schemas"]["AgentTask"];
export type AgentAdmissionEvent = components["schemas"]["AgentAdmissionEvent"];
export type AgentGrants = components["schemas"]["AgentGrants"];
export type AgentCancelReceipt = components["schemas"]["AgentCancelReceipt"];
export type CreateAgentRequest = components["schemas"]["CreateAgentRequest"];
export type Sandbox = components["schemas"]["Sandbox"];
export type ToolSuiteDefinition=components["schemas"]["ToolSuiteDefinition"];
export type SandboxToolSuite=components["schemas"]["SandboxToolSuite"];
export type SSHJobRecoveryView=components["schemas"]["SSHJobRecoveryView"];
export type SSHJobRecoveryRecord=components["schemas"]["SSHJobRecoveryRecord"];
export type ResolveSSHJobRecoveryRequest=components["schemas"]["ResolveSSHJobRecoveryRequest"];
export type SSHJobResolutionReceipt=components["schemas"]["SSHJobResolutionReceipt"];
export type SSHJobAdmissionState=components["schemas"]["SSHJobAdmissionState"];
export type SetSSHJobAdmissionRequest=components["schemas"]["SetSSHJobAdmissionRequest"];
export type SandboxActivity = components["schemas"]["SandboxActivity"];
export type SandboxActivityEvent = components["schemas"]["SandboxActivityEvent"];
export type SandboxProfileGrants = components["schemas"]["SandboxProfileGrants"];
export type SandboxProfileGrant = components["schemas"]["SandboxProfileGrant"];
export type SandboxProfileCapability = components["schemas"]["SandboxProfileCapability"];
export type SandboxProfileGrantEvents = components["schemas"]["SandboxProfileGrantEvents"];
export type SandboxProfileGrantEvent = components["schemas"]["SandboxProfileGrantEvent"];
export type ReplaceSandboxProfileGrantsRequest = components["schemas"]["ReplaceSandboxProfileGrantsRequest"];
export type CreateLocalSandboxRequest = components["schemas"]["CreateLocalSandboxRequest"];
export type CreateDockerSandboxRequest = components["schemas"]["CreateDockerSandboxRequest"];
export type CachedDockerImage = components["schemas"]["CachedDockerImage"];
export type CreateSSHRemoteSandboxRequest = components["schemas"]["CreateSSHRemoteSandboxRequest"];
export type SSHHostIdentity = components["schemas"]["SSHHostIdentity"];
export type CreateDaytonaSandboxRequest = components["schemas"]["CreateDaytonaSandboxRequest"];
export type SandboxBackends = components["schemas"]["SandboxBackends"];
export type CreateOpenAITunnelRequest =
  components["schemas"]["CreateOpenAITunnelRequest"];
export type AuditRecord = components["schemas"]["AuditRecord"];
export type ProviderActivationAuditRecord =
  components["schemas"]["ProviderActivationAuditRecord"];
export type SystemStatus = components["schemas"]["SystemStatus"];
export type RuntimeGovernanceStatus = components["schemas"]["RuntimeGovernanceStatus"];
export type RuntimeBudget = components["schemas"]["RuntimeBudget"];
export type RuntimeIsolation = components["schemas"]["RuntimeIsolation"];
export type PolicyStatus = components["schemas"]["PolicyStatus"];
export type Policy = components["schemas"]["Policy"];
export type PolicyDecision = components["schemas"]["PolicyDecision"];
export type PolicyRule = components["schemas"]["PolicyRule"];
export type PolicyRuleInput = components["schemas"]["PolicyRuleInput"];
export type PolicyMatch = components["schemas"]["PolicyMatch"];
export type PolicyResourceMatcher =
  components["schemas"]["PolicyResourceMatcher"];
export type PolicyDefaultRequest =
  components["schemas"]["PolicyDefaultRequest"];
export type CredentialStatus = components["schemas"]["CredentialStatus"];
export type Credential = components["schemas"]["Credential"];
export type StartOAuthCredentialRequest = components["schemas"]["StartOAuthCredentialRequest"];
export type OAuthFlowStart = components["schemas"]["OAuthFlowStart"];
export type OAuthFlow = components["schemas"]["OAuthFlow"];
export type CreateNativeCredentialRequest =
  components["schemas"]["CreateNativeCredentialRequest"];
export type CreateEnvironmentCredentialRequest =
  components["schemas"]["CreateEnvironmentCredentialRequest"];
export type RotateCredentialRequest =
  components["schemas"]["RotateCredentialRequest"];
export type MCPServerInstance = components["schemas"]["MCPServerInstance"];
export type StartMCPServerRequest =
  components["schemas"]["StartMCPServerRequest"];
export type ProviderMutationResponse =
  components["schemas"]["ProviderMutationResponse"];
export type CreateProviderRequest = components["schemas"]["CreateProviderRequest"];
export type RebindProviderRequest = components["schemas"]["RebindProviderRequest"];
export type UpdateProviderRequest =
  components["schemas"]["UpdateProviderRequest"];
export type CreateProfileRequest = components["schemas"]["CreateProfileRequest"];
export type ProfileMCPDelivery = components["schemas"]["ProfileMCPDelivery"];
export type ConfigureProfileMCPDeliveryRequest = components["schemas"]["ConfigureProfileMCPDeliveryRequest"];
export type AddProfileToolRequest =
  components["schemas"]["AddProfileToolRequest"];
export type ReplaceProfileToolsRequest =
  components["schemas"]["ReplaceProfileToolsRequest"];
export type ReplaceProfileSkillsRequest =
  components["schemas"]["ReplaceProfileSkillsRequest"];

export interface BrowseToolsInput {
  limit?: number;
  sourceId?: string;
  cursor?: string;
}

export interface SearchToolsInput {
  cursor?: string;
  query: string;
  limit?: number;
  sourceId?: string;
  profile?: string;
}

export interface ManagementClient {
  getSystemStatus(): Promise<SystemStatus>;
  listPluginCatalogue(): Promise<{plugins: PluginCatalogueEntry[]}>;
  enablePluginInstallation(installationId:string):Promise<{installation:ProviderPluginInstallation}>;
  disablePluginInstallation(installationId:string):Promise<{installation:ProviderPluginInstallation}>;
  deletePluginInstallation(installationId:string):Promise<void>;
  listProviderPlugins(): Promise<{
    plugins: ProviderPlugin[];
    issues: ProviderPluginIssue[];
  }>;
  listProviderPluginInstallations(): Promise<{
    installations: ProviderPluginInstallation[];
  }>;
  enableProviderPluginInstallation(
    installationId: string,
  ): Promise<{ installation: ProviderPluginInstallation }>;
  disableProviderPluginInstallation(
    installationId: string,
  ): Promise<{ installation: ProviderPluginInstallation }>;
  deleteProviderPluginInstallation(installationId: string): Promise<void>;
  listSkills(): Promise<{ installations: SkillInstallation[] }>;
  getSkillInstallation(installationId: string): Promise<SkillInspection>;
  trustSkillInstallation(
    installationId: string,
  ): Promise<{ installation: SkillInstallation }>;
  untrustSkillInstallation(
    installationId: string,
  ): Promise<{ installation: SkillInstallation }>;
  enableSkillInstallation(
    installationId: string,
  ): Promise<{ installation: SkillInstallation }>;
  disableSkillInstallation(
    installationId: string,
  ): Promise<{ installation: SkillInstallation }>;
  deleteSkillInstallation(installationId: string): Promise<void>;
  listSkillDeployments(): Promise<{ deployments: SkillDeployment[] }>;
  undeploySkill(deploymentId: string): Promise<void>;
  listProviders(): Promise<{ providers: Provider[] }>;
  createProvider(body: CreateProviderRequest): Promise<ProviderMutationResponse>;
  updateProvider(
    providerId: string,
    body: UpdateProviderRequest,
  ): Promise<ProviderMutationResponse>;
  rebindProvider(
    providerId: string,
    body: RebindProviderRequest,
  ): Promise<ProviderMutationResponse>;
  syncProvider(providerId: string): Promise<SyncResult>;
  deleteProvider(providerId: string): Promise<void>;
  browseTools(input?: BrowseToolsInput): Promise<{ tools: ToolSummary[]; nextCursor?: string }>;
  searchTools(input: SearchToolsInput): Promise<{ tools: ToolSummary[]; nextCursor?: string }>;
  getTool(toolId: string): Promise<ToolDetail>;
  callTool(
    toolId: string,
    argumentsValue: Record<string, unknown>,
  ): Promise<InvocationResult>;
  listProfiles(): Promise<{ profiles: Profile[] }>;
  createProfile(body: CreateProfileRequest): Promise<Profile>;
  getProfile(slug: string): Promise<Profile>;
  addProfileTool(slug: string, body: AddProfileToolRequest): Promise<Profile>;
  replaceProfileTools(
    slug: string,
    body: ReplaceProfileToolsRequest,
  ): Promise<Profile>;
  removeProfileTool(slug: string, toolId: string): Promise<Profile>;
  replaceProfileSkills(slug: string, body: ReplaceProfileSkillsRequest): Promise<Profile>;
  listProfileSkillDeployments(slug: string): Promise<{ deployments: SkillDeployment[] }>;
  undeployProfileSkill(slug: string, deploymentId: string): Promise<void>;
  getProfileMCPDelivery(slug: string): Promise<ProfileMCPDelivery | null>;
  configureProfileMCPDelivery(slug: string, body: ConfigureProfileMCPDeliveryRequest): Promise<ProfileMCPDelivery>;
  listIntegrationOfferings(): Promise<{integrations:IntegrationOffering[]}>;
  listAgents(): Promise<{agents:Agent[]}>;
  getAgent(agentId:string): Promise<Agent>;
  getAgentTools(agentId:string): Promise<AgentTools>;
  ensureAgentTools(agentId:string): Promise<AgentTools>;
  createAgent(body:CreateAgentRequest): Promise<Agent>;
  updateAgentSettings(agentId:string,defaultModel:string,defaultReasoningEffort:string):Promise<Agent>;
  inspectAgentModels(agentId:string):Promise<AgentModelChoices>;
  listAgentWorkspaces(agentId:string):Promise<{workspaces:AgentWorkspace[]}>;
  bindAgentWorkspace(agentId:string,key:string,workspaceRoot:string):Promise<void>;
  unbindAgentWorkspace(agentId:string,key:string):Promise<void>;
  grantAgentWorkspace(agentId:string,key:string,profileId:string):Promise<void>;
  revokeAgentWorkspace(agentId:string,key:string,profileId:string):Promise<void>;
  deleteAgent(agentId:string): Promise<void>;
  listAgentTasks(agentId:string): Promise<{tasks:AgentTask[]}>;
  listAgentAdmissions(agentId:string):Promise<{events:AgentAdmissionEvent[]}>;
  getAgentTask(taskId:string): Promise<AgentTask>;
  cancelAgentTask(taskId:string): Promise<AgentCancelReceipt>;
  listAgentGrants(agentId:string): Promise<AgentGrants>;
  grantAgentProfile(agentId:string,profileId:string): Promise<void>;
  revokeAgentProfile(agentId:string,profileId:string): Promise<void>;
  listToolSuites(): Promise<{suites:ToolSuiteDefinition[]}>;
  getSandboxTools(sandboxId:string):Promise<SandboxToolSuite>;
  getSandboxJobTools(sandboxId:string):Promise<SandboxToolSuite>;
  getSSHJobRecovery(sandboxId:string):Promise<SSHJobRecoveryView>;
  resolveSSHJobRecovery(sandboxId:string,taskId:string,body:ResolveSSHJobRecoveryRequest):Promise<SSHJobResolutionReceipt>;
  getSSHJobAdmission(sandboxId:string):Promise<SSHJobAdmissionState>;
  setSSHJobAdmission(sandboxId:string,body:SetSSHJobAdmissionRequest):Promise<SSHJobAdmissionState>;
  listSandboxes(): Promise<{ sandboxes: Sandbox[] }>;
  getSandbox(sandboxId: string): Promise<Sandbox>;
  getSandboxActivity(sandboxId: string, limit?: number): Promise<SandboxActivity>;
  getSandboxProfileGrants(sandboxId: string): Promise<SandboxProfileGrants>;
  putSandboxProfileGrants(sandboxId: string, profileId: string, body: ReplaceSandboxProfileGrantsRequest): Promise<SandboxProfileGrants>;
  revokeSandboxProfileGrants(sandboxId: string, profileId: string): Promise<SandboxProfileGrants>;
  getSandboxProfileGrantEvents(sandboxId: string): Promise<SandboxProfileGrantEvents>;
  createLocalSandbox(body: CreateLocalSandboxRequest): Promise<Sandbox>;
  createDockerSandbox(body: CreateDockerSandboxRequest): Promise<Sandbox>;
  createDaytonaSandbox(body: CreateDaytonaSandboxRequest): Promise<Sandbox>;
  createSSHRemoteSandbox(body: CreateSSHRemoteSandboxRequest): Promise<Sandbox>;
  inspectSSHHostKey(host: string, port: number): Promise<SSHHostIdentity>;
  createExternalSandbox(body:CreateExternalSandboxRequest):Promise<Sandbox>;
  reconcileSandbox(sandboxId:string):Promise<Sandbox>;
  reconcileDaytonaSandbox(sandboxId: string): Promise<Sandbox>;
  getSandboxBackends(): Promise<SandboxBackends>;
  listDockerImages(): Promise<{ images: CachedDockerImage[] }>;
  reconcileDockerSandbox(sandboxId: string): Promise<Sandbox>;
  startSandbox(sandboxId: string): Promise<Sandbox>;
  stopSandbox(sandboxId: string): Promise<Sandbox>;
  destroySandbox(sandboxId: string): Promise<Sandbox>;
  listTunnels(): Promise<{ tunnels: Tunnel[] }>;
  createOpenAITunnel(body: CreateOpenAITunnelRequest): Promise<Tunnel>;
  createExternalTunnel(body:CreateExternalTunnelRequest):Promise<Tunnel>;
  deleteTunnel(tunnelId: string): Promise<void>;
  listMCPServers(): Promise<{ servers: MCPServerInstance[] }>;
  startMCPServer(body: StartMCPServerRequest): Promise<MCPServerInstance>;
  restartMCPServer(serverId: string): Promise<MCPServerInstance>;
  stopMCPServer(serverId: string): Promise<void>;
  listAudit(limit?: number): Promise<{ records: AuditRecord[] }>;
  listProviderActivationAudit(
    limit?: number,
  ): Promise<{ records: ProviderActivationAuditRecord[] }>;
  getRuntimeGovernance(): Promise<RuntimeGovernanceStatus>;
  updateRuntimeBudget(body: RuntimeBudget): Promise<RuntimeGovernanceStatus>;
  updateRuntimeIsolation(body: RuntimeIsolation): Promise<RuntimeGovernanceStatus>;
  getPolicyStatus(): Promise<PolicyStatus>;
  getPolicy(): Promise<Policy>;
  setPolicyDefault(body: PolicyDefaultRequest): Promise<Policy>;
  createPolicyRule(body: PolicyRuleInput): Promise<PolicyRule>;
  updatePolicyRule(ruleId: string, body: PolicyRuleInput): Promise<PolicyRule>;
  deletePolicyRule(ruleId: string): Promise<void>;
  setPolicyRuleEnabled(ruleId: string, enabled: boolean): Promise<PolicyRule>;
  getCredentialStatus(): Promise<CredentialStatus>;
  listCredentials(): Promise<{ credentials: Credential[] }>;
  getCredential(namespace: string, name: string): Promise<Credential>;
  createNativeCredential(body: CreateNativeCredentialRequest): Promise<Credential>;
  createEnvironmentCredential(
    body: CreateEnvironmentCredentialRequest,
  ): Promise<Credential>;
  startOAuthCredential(body: StartOAuthCredentialRequest): Promise<OAuthFlowStart>;
  getOAuthFlow(flowId: string): Promise<OAuthFlow>;
  cancelOAuthFlow(flowId: string): Promise<void>;
  rotateCredential(
    namespace: string,
    name: string,
    body: RotateCredentialRequest,
  ): Promise<Credential>;
  disconnectOAuthCredential(namespace: string, name: string): Promise<Credential>;
  deleteCredential(namespace: string, name: string): Promise<void>;
}
