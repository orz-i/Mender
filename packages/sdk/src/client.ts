import createClient from "openapi-fetch";

import type { components, paths } from "./generated/schema";

export type PluginCatalogueEntry=components["schemas"]["PluginCatalogueEntry"];
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
export type AgentTask = components["schemas"]["AgentTask"];
export type AgentAdmissionEvent = components["schemas"]["AgentAdmissionEvent"];
export type AgentGrants = components["schemas"]["AgentGrants"];
export type AgentCancelReceipt = components["schemas"]["AgentCancelReceipt"];
export type CreateAgentRequest = components["schemas"]["CreateAgentRequest"];
export type Sandbox = components["schemas"]["Sandbox"];
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
export type CreateDaytonaSandboxRequest = components["schemas"]["CreateDaytonaSandboxRequest"];
export type SandboxBackends = components["schemas"]["SandboxBackends"];
export type CreateOpenAITunnelRequest =
  components["schemas"]["CreateOpenAITunnelRequest"];
export type SetProfileTunnelRequest =
  components["schemas"]["SetProfileTunnelRequest"];
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
export type CreateHTTPProviderRequest =
  components["schemas"]["CreateHTTPProviderRequest"];
export type CreateStdioProviderRequest =
  components["schemas"]["CreateStdioProviderRequest"];
export type UpdateProviderRequest =
  components["schemas"]["UpdateProviderRequest"];
export type UpdateConfiguredProviderRequest =
  components["schemas"]["UpdateConfiguredProviderRequest"];
export type CreateProfileRequest = components["schemas"]["CreateProfileRequest"];
export type AddProfileToolRequest =
  components["schemas"]["AddProfileToolRequest"];
export type ReplaceProfileToolsRequest =
  components["schemas"]["ReplaceProfileToolsRequest"];

export interface BrowseToolsInput {
  limit?: number;
  providerId?: string;
}

export interface SearchToolsInput {
  query: string;
  limit?: number;
  providerId?: string;
  profile?: string;
}

export interface MenderClient {
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
  createHTTPProvider(
    body: CreateHTTPProviderRequest,
  ): Promise<ProviderMutationResponse>;
  createStdioProvider(
    body: CreateStdioProviderRequest,
  ): Promise<ProviderMutationResponse>;
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
  browseTools(input?: BrowseToolsInput): Promise<{ tools: ToolSummary[] }>;
  searchTools(input: SearchToolsInput): Promise<{ tools: ToolSummary[] }>;
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
  setProfileTunnel(slug: string, body: SetProfileTunnelRequest): Promise<Profile>;
  listAgents(): Promise<{agents:Agent[]}>;
  getAgent(agentId:string): Promise<Agent>;
  createAgent(body:CreateAgentRequest): Promise<Agent>;
  deleteAgent(agentId:string): Promise<void>;
  listAgentTasks(agentId:string): Promise<{tasks:AgentTask[]}>;
  listAgentAdmissions(agentId:string):Promise<{events:AgentAdmissionEvent[]}>;
  getAgentTask(taskId:string): Promise<AgentTask>;
  cancelAgentTask(taskId:string): Promise<AgentCancelReceipt>;
  listAgentGrants(agentId:string): Promise<AgentGrants>;
  grantAgentProfile(agentId:string,profileId:string): Promise<void>;
  revokeAgentProfile(agentId:string,profileId:string): Promise<void>;
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
  rotateCredential(
    namespace: string,
    name: string,
    body: RotateCredentialRequest,
  ): Promise<Credential>;
  deleteCredential(namespace: string, name: string): Promise<void>;
}

type FetchLike = (input: Request) => Promise<Response>;

export class MenderApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "MenderApiError";
    this.status = status;
    this.code = code;
  }
}

export interface MenderClientOptions {
  baseUrl?: string;
  fetch?: FetchLike;
}

export function createMenderClient(
  options: MenderClientOptions = {},
): MenderClient {
  const client = createClient<paths>({
    baseUrl: options.baseUrl ?? "/api/v1",
    ...(options.fetch === undefined ? {} : { fetch: options.fetch }),
  });

  return {
    async getSystemStatus() {
      const result = await client.GET("/system/status");
      return requireData(result);
    },

    async listPluginCatalogue(){
      return requireData(await client.GET("/plugins"));
    },
    async enablePluginInstallation(installationId:string){
      return requireData(await client.POST("/plugin-installations/{installationId}/enable",{params:{path:{installationId}}}));
    },
    async disablePluginInstallation(installationId:string){
      return requireData(await client.POST("/plugin-installations/{installationId}/disable",{params:{path:{installationId}}}));
    },
    async deletePluginInstallation(installationId:string){
      const result=await client.DELETE("/plugin-installations/{installationId}",{params:{path:{installationId}}});
      if(!result.response.ok)throwApiError(result.error,result.response);
    },
    async listProviderPlugins() {
      const result = await client.GET("/provider-plugins");
      return requireData(result);
    },

    async listProviderPluginInstallations() {
      const result = await client.GET("/provider-plugin-installations");
      return requireData(result);
    },

    async enableProviderPluginInstallation(installationId: string) {
      const result = await client.POST(
        "/provider-plugin-installations/{installationId}/enable",
        { params: { path: { installationId } } },
      );
      return requireData(result);
    },

    async disableProviderPluginInstallation(installationId: string) {
      const result = await client.POST(
        "/provider-plugin-installations/{installationId}/disable",
        { params: { path: { installationId } } },
      );
      return requireData(result);
    },

    async deleteProviderPluginInstallation(installationId: string) {
      const result = await client.DELETE(
        "/provider-plugin-installations/{installationId}",
        { params: { path: { installationId } } },
      );
      if (!result.response.ok) {
        throwApiError(result.error, result.response);
      }
    },

    async listSkills() {
      const result = await client.GET("/skills");
      return requireData(result);
    },

    async getSkillInstallation(installationId: string) {
      const result = await client.GET("/skills/{installationId}", {
        params: { path: { installationId } },
      });
      return requireData(result);
    },

    async trustSkillInstallation(installationId: string) {
      const result = await client.POST("/skills/{installationId}/trust", {
        params: { path: { installationId } },
      });
      return requireData(result);
    },

    async untrustSkillInstallation(installationId: string) {
      const result = await client.POST("/skills/{installationId}/untrust", {
        params: { path: { installationId } },
      });
      return requireData(result);
    },

    async enableSkillInstallation(installationId: string) {
      const result = await client.POST("/skills/{installationId}/enable", {
        params: { path: { installationId } },
      });
      return requireData(result);
    },

    async disableSkillInstallation(installationId: string) {
      const result = await client.POST("/skills/{installationId}/disable", {
        params: { path: { installationId } },
      });
      return requireData(result);
    },

    async deleteSkillInstallation(installationId: string) {
      const result = await client.DELETE("/skills/{installationId}", {
        params: { path: { installationId } },
      });
      if (!result.response.ok) {
        throwApiError(result.error, result.response);
      }
    },

    async listSkillDeployments() {
      const result = await client.GET("/skill-deployments");
      return requireData(result);
    },

    async undeploySkill(deploymentId: string) {
      const result = await client.DELETE(
        "/skill-deployments/{deploymentId}",
        { params: { path: { deploymentId } } },
      );
      if (!result.response.ok) {
        throwApiError(result.error, result.response);
      }
    },

    async listProviders() {
      const result = await client.GET("/providers");
      return requireData(result);
    },

    async createProvider(body: CreateProviderRequest) {
      const result = await client.POST("/providers", { body });
      return requireData(result);
    },

    async createHTTPProvider(body: CreateHTTPProviderRequest) {
      const result = await client.POST("/providers/http", { body });
      return requireData(result);
    },

    async createStdioProvider(body: CreateStdioProviderRequest) {
      const result = await client.POST("/providers/stdio", { body });
      return requireData(result);
    },

    async updateProvider(providerId: string, body: UpdateProviderRequest) {
      const result = await client.PUT("/providers/{providerId}", {
        params: { path: { providerId } },
        body,
      });
      return requireData(result);
    },

    async rebindProvider(providerId: string, body: RebindProviderRequest) {
      const result = await client.POST("/providers/{providerId}/rebind", {
        params: { path: { providerId } },
        body,
      });
      return requireData(result);
    },

    async syncProvider(providerId: string) {
      const result = await client.POST("/providers/{providerId}/sync", {
        params: { path: { providerId } },
      });
      return requireData(result);
    },

    async deleteProvider(providerId: string) {
      const result = await client.DELETE("/providers/{providerId}", {
        params: { path: { providerId } },
      });
      if (!result.response.ok) {
        throwApiError(result.error, result.response);
      }
    },

    async browseTools(input: BrowseToolsInput = {}) {
      const result = await client.GET("/tools/browse", {
        params: {
          query: {
            ...(input.limit === undefined ? {} : { limit: input.limit }),
            ...(input.providerId === undefined
              ? {}
              : { providerId: input.providerId }),
          },
        },
      });
      return requireData(result);
    },

    async searchTools(input: SearchToolsInput) {
      const result = await client.GET("/tools", {
        params: {
          query: {
            query: input.query,
            ...(input.limit === undefined ? {} : { limit: input.limit }),
            ...(input.providerId === undefined
              ? {}
              : { providerId: input.providerId }),
            ...(input.profile === undefined ? {} : { profile: input.profile }),
          },
        },
      });
      return requireData(result);
    },

    async getTool(toolId: string) {
      const result = await client.GET("/tools/{toolId}", {
        params: { path: { toolId } },
      });
      return requireData(result);
    },

    async callTool(toolId: string, argumentsValue: Record<string, unknown>) {
      const result = await client.POST("/tools/{toolId}/call", {
        params: { path: { toolId } },
        body: { arguments: argumentsValue },
      });
      return requireData(result);
    },

    async listProfiles() {
      const result = await client.GET("/profiles");
      return requireData(result);
    },

    async createProfile(body: CreateProfileRequest) {
      const result = await client.POST("/profiles", { body });
      return requireData(result);
    },

    async getProfile(slug: string) {
      const result = await client.GET("/profiles/{slug}", {
        params: { path: { slug } },
      });
      return requireData(result);
    },

    async addProfileTool(slug: string, body: AddProfileToolRequest) {
      const result = await client.POST("/profiles/{slug}/tools", {
        params: { path: { slug } },
        body,
      });
      return requireData(result);
    },

    async replaceProfileTools(
      slug: string,
      body: ReplaceProfileToolsRequest,
    ) {
      const result = await client.PUT("/profiles/{slug}/tools", {
        params: { path: { slug } },
        body,
      });
      return requireData(result);
    },

    async removeProfileTool(slug: string, toolId: string) {
      const result = await client.DELETE("/profiles/{slug}/tools/{toolId}", {
        params: { path: { slug, toolId } },
      });
      return requireData(result);
    },

    async setProfileTunnel(slug: string, body: SetProfileTunnelRequest) {
      const result = await client.PUT("/profiles/{slug}/tunnel", {
        params: { path: { slug } },
        body,
      });
      return requireData(result);
    },

    async listAgents(){return requireData(await client.GET("/agents"));},
    async getAgent(agentId){return requireData(await client.GET("/agents/{agentId}",{params:{path:{agentId}}}));},
    async createAgent(body){return requireData(await client.POST("/agents",{body}));},
    async deleteAgent(agentId){
      const result=await client.DELETE("/agents/{agentId}",{params:{path:{agentId}}});
      if(!result.response.ok)throwApiError(result.error,result.response);
    },
    async listAgentTasks(agentId){return requireData(await client.GET("/agents/{agentId}/tasks",{params:{path:{agentId}}}));},
    async listAgentAdmissions(agentId){return requireData(await client.GET("/agents/{agentId}/admissions",{params:{path:{agentId}}}));},
    async getAgentTask(taskId){return requireData(await client.GET("/agent-tasks/{taskId}",{params:{path:{taskId}}}));},
    async cancelAgentTask(taskId){return requireData(await client.POST("/agent-tasks/{taskId}/cancel",{params:{path:{taskId}}}));},
    async listAgentGrants(agentId){return requireData(await client.GET("/agents/{agentId}/profile-grants",{params:{path:{agentId}}}));},
    async grantAgentProfile(agentId,profileId){
      const result=await client.PUT("/agents/{agentId}/profile-grants/{profileId}",{params:{path:{agentId,profileId}}});
      if(!result.response.ok)throwApiError(result.error,result.response);
    },
    async revokeAgentProfile(agentId,profileId){
      const result=await client.DELETE("/agents/{agentId}/profile-grants/{profileId}",{params:{path:{agentId,profileId}}});
      if(!result.response.ok)throwApiError(result.error,result.response);
    },
    async listSandboxes() {
      const result = await client.GET("/sandboxes");
      return requireData(result);
    },

    async getSandbox(sandboxId: string) {
      const result = await client.GET("/sandboxes/{sandboxId}", {params: {path: {sandboxId}}});
      return requireData(result);
    },

    async getSandboxActivity(sandboxId, limit = 50) {
      const result = await client.GET("/sandboxes/{sandboxId}/activity", {params:{path:{sandboxId},query:{limit}}});
      return requireData(result);
    },
    async getSandboxProfileGrants(sandboxId) {
      return requireData(await client.GET("/sandboxes/{sandboxId}/profile-grants", { params: { path: { sandboxId } } }));
    },
    async putSandboxProfileGrants(sandboxId, profileId, body) {
      return requireData(await client.PUT("/sandboxes/{sandboxId}/profile-grants/{profileId}", {
        params: { path: { sandboxId, profileId } }, body,
      }));
    },
    async revokeSandboxProfileGrants(sandboxId, profileId) {
      return requireData(await client.DELETE("/sandboxes/{sandboxId}/profile-grants/{profileId}", {
        params: { path: { sandboxId, profileId } },
      }));
    },
    async getSandboxProfileGrantEvents(sandboxId) {
      return requireData(await client.GET("/sandboxes/{sandboxId}/profile-grant-events", { params: { path: { sandboxId } } }));
    },
    async createLocalSandbox(body: CreateLocalSandboxRequest) {
      const result = await client.POST("/sandboxes/local", {body});
      return requireData(result);
    },

    async createDockerSandbox(body: CreateDockerSandboxRequest) {
      const result = await client.POST("/sandboxes/docker", {body});
      return requireData(result);
    },
    async createExternalSandbox(body:CreateExternalSandboxRequest){
      return requireData(await client.POST("/sandboxes/external",{body}));
    },
    async reconcileSandbox(sandboxId:string){
      return requireData(await client.POST("/sandboxes/{sandboxId}/reconcile",{params:{path:{sandboxId}}}));
    },
    async createDaytonaSandbox(body: CreateDaytonaSandboxRequest) {
      const result = await client.POST("/sandboxes/daytona", {body});
      return requireData(result);
    },
    async createSSHRemoteSandbox(body: CreateSSHRemoteSandboxRequest) {
      const result = await client.POST("/sandboxes/ssh-remote", {body});
      return requireData(result);
    },
    async reconcileDaytonaSandbox(sandboxId: string) {
      const result = await client.POST("/sandboxes/{sandboxId}/daytona/reconcile", {params:{path:{sandboxId}}});
      return requireData(result);
    },
    async getSandboxBackends() {
      const result = await client.GET("/sandboxes/backends");
      return requireData(result);
    },
    async listDockerImages() {
      const result = await client.GET("/sandboxes/docker/images");
      return requireData(result);
    },
    async reconcileDockerSandbox(sandboxId: string) {
      const result = await client.POST("/sandboxes/{sandboxId}/reconcile", {params:{path:{sandboxId}}});
      return requireData(result);
    },

    async startSandbox(sandboxId: string) {
      const result = await client.POST("/sandboxes/{sandboxId}/start", {params:{path:{sandboxId}}});
      return requireData(result);
    },
    async stopSandbox(sandboxId: string) {
      const result = await client.POST("/sandboxes/{sandboxId}/stop", {params:{path:{sandboxId}}});
      return requireData(result);
    },
    async destroySandbox(sandboxId: string) {
      const result = await client.DELETE("/sandboxes/{sandboxId}", {params:{path:{sandboxId}}});
      return requireData(result);
    },

    async listTunnels() {
      const result = await client.GET("/tunnels");
      return requireData(result);
    },

    async createExternalTunnel(body:CreateExternalTunnelRequest){
      return requireData(await client.POST("/tunnels/external",{body}));
    },
    async createOpenAITunnel(body: CreateOpenAITunnelRequest) {
      const result = await client.POST("/tunnels/openai", { body });
      return requireData(result);
    },

    async deleteTunnel(tunnelId: string) {
      const result = await client.DELETE("/tunnels/{tunnelId}", {
        params: { path: { tunnelId } },
      });
      if (!result.response.ok) {
        throwApiError(result.error, result.response);
      }
    },

    async listMCPServers() {
      const result = await client.GET("/mcp/servers");
      return requireData(result);
    },

    async startMCPServer(body: StartMCPServerRequest) {
      const result = await client.POST("/mcp/servers", { body });
      return requireData(result);
    },

    async restartMCPServer(serverId: string) {
      const result = await client.POST("/mcp/servers/{serverId}/restart", {
        params: { path: { serverId } },
      });
      return requireData(result);
    },

    async stopMCPServer(serverId: string) {
      const result = await client.DELETE("/mcp/servers/{serverId}", {
        params: { path: { serverId } },
      });
      if (!result.response.ok) {
        throwApiError(result.error, result.response);
      }
    },

    async listAudit(limit = 50) {
      const result = await client.GET("/audit", {
        params: { query: { limit } },
      });
      return requireData(result);
    },

    async listProviderActivationAudit(limit = 50) {
      const result = await client.GET("/audit/provider-activations", {
        params: { query: { limit } },
      });
      return requireData(result);
    },

    async getRuntimeGovernance() {
      const result = await client.GET("/runtime-governance");
      return requireData(result);
    },

    async updateRuntimeBudget(body: RuntimeBudget) {
      const result = await client.PUT("/runtime-governance/budget", { body });
      return requireData(result);
    },

    async updateRuntimeIsolation(body: RuntimeIsolation) {
      const result = await client.PUT("/runtime-governance/isolation", { body });
      return requireData(result);
    },

    async getPolicyStatus() {
      const result = await client.GET("/policy/status");
      return requireData(result);
    },

    async getPolicy() {
      const result = await client.GET("/policy");
      return requireData(result);
    },

    async setPolicyDefault(body: PolicyDefaultRequest) {
      const result = await client.PUT("/policy/default", { body });
      return requireData(result);
    },

    async createPolicyRule(body: PolicyRuleInput) {
      const result = await client.POST("/policy/rules", { body });
      return requireData(result);
    },

    async updatePolicyRule(ruleId: string, body: PolicyRuleInput) {
      const result = await client.PUT("/policy/rules/{ruleId}", {
        params: { path: { ruleId } },
        body,
      });
      return requireData(result);
    },

    async deletePolicyRule(ruleId: string) {
      const result = await client.DELETE("/policy/rules/{ruleId}", {
        params: { path: { ruleId } },
      });
      if (!result.response.ok) {
        throwApiError(result.error, result.response);
      }
    },

    async setPolicyRuleEnabled(ruleId: string, enabled: boolean) {
      const result = enabled
        ? await client.POST("/policy/rules/{ruleId}/enable", {
            params: { path: { ruleId } },
          })
        : await client.POST("/policy/rules/{ruleId}/disable", {
            params: { path: { ruleId } },
          });
      return requireData(result);
    },

    async getCredentialStatus() {
      const result = await client.GET("/credentials/status");
      return requireData(result);
    },

    async listCredentials() {
      const result = await client.GET("/credentials");
      return requireData(result);
    },

    async getCredential(namespace: string, name: string) {
      const result = await client.GET("/credentials/{namespace}/{name}", {
        params: { path: { namespace, name } },
      });
      return requireData(result);
    },

    async createNativeCredential(body: CreateNativeCredentialRequest) {
      const result = await client.POST("/credentials/native", { body });
      return requireData(result);
    },

    async createEnvironmentCredential(body: CreateEnvironmentCredentialRequest) {
      const result = await client.POST("/credentials/environment", { body });
      return requireData(result);
    },

    async rotateCredential(
      namespace: string,
      name: string,
      body: RotateCredentialRequest,
    ) {
      const result = await client.POST("/credentials/{namespace}/{name}/rotate", {
        params: { path: { namespace, name } },
        body,
      });
      return requireData(result);
    },

    async deleteCredential(namespace: string, name: string) {
      const result = await client.DELETE("/credentials/{namespace}/{name}", {
        params: { path: { namespace, name } },
      });
      if (!result.response.ok) {
        throwApiError(result.error, result.response);
      }
    },
  };
}

type ApiResult<T> = {
  data?: T;
  error?: unknown;
  response: Response;
};

function requireData<T>(result: ApiResult<T>): T {
  if (result.data !== undefined) {
    return result.data;
  }
  throwApiError(result.error, result.response);
}

function throwApiError(error: unknown, response: Response): never {
  if (
    typeof error === "object" &&
    error !== null &&
    "code" in error &&
    "message" in error &&
    typeof error.code === "string" &&
    typeof error.message === "string"
  ) {
    throw new MenderApiError(response.status, error.code, error.message);
  }
  throw new MenderApiError(
    response.status,
    "http_error",
    `Mender API request failed with HTTP ${response.status}`,
  );
}
