import createClient from "openapi-fetch";

import type {
  AddProfileToolRequest,
  BrowseToolsInput,
  ConfigureProfileMCPDeliveryRequest,
  CreateDaytonaSandboxRequest,
  CreateDockerSandboxRequest,
  CreateEnvironmentCredentialRequest,
  CreateExternalSandboxRequest,
  CreateExternalTunnelRequest,
  CreateLocalSandboxRequest,
  CreateNativeCredentialRequest,
  CreateOpenAITunnelRequest,
  CreateProfileRequest,
  CreateProviderRequest,
  CreateSSHRemoteSandboxRequest,
  ManagementClient,
  PolicyDefaultRequest,
  PolicyRuleInput,
  RebindProviderRequest,
  ReplaceProfileSkillsRequest,
  ReplaceProfileToolsRequest,
  RotateCredentialRequest,
  RuntimeBudget,
  RuntimeIsolation,
  SearchToolsInput,
  StartMCPServerRequest,
  StartOAuthCredentialRequest,
  UpdateProviderRequest,
} from "./contracts/management-client";
import type { paths } from "./generated/schema";

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
): ManagementClient {
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
            ...(input.sourceId === undefined
              ? {}
              : { sourceId: input.sourceId }),
            ...(input.cursor === undefined ? {} : { cursor: input.cursor }),
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
            ...(input.sourceId === undefined
              ? {}
              : { sourceId: input.sourceId }),
            ...(input.cursor === undefined ? {} : { cursor: input.cursor }),
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

    async undeployProfileSkill(slug: string, deploymentId: string) {
      const result = await client.DELETE("/profiles/{slug}/skill-deployments/{deploymentId}", {
        params: { path: { slug, deploymentId } },
      });
      if (!result.response.ok) throwApiError(result.error, result.response);
    },

    async listProfileSkillDeployments(slug: string) {
      const result = await client.GET("/profiles/{slug}/skill-deployments", {
        params: { path: { slug } },
      });
      return requireData(result);
    },

    async replaceProfileSkills(slug: string, body: ReplaceProfileSkillsRequest) {
      const result = await client.PUT("/profiles/{slug}/skills", {
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

    async getProfileMCPDelivery(slug: string) {
      const result = await client.GET("/profiles/{slug}/mcp-delivery", {params:{path:{slug}}});
      if(result.response.status===404&&result.error?.code==="mcp_delivery_not_configured")return null;
      return requireData(result);
    },

    async configureProfileMCPDelivery(slug: string, body: ConfigureProfileMCPDeliveryRequest) {
      const result = await client.PUT("/profiles/{slug}/mcp-delivery", {
        params: { path: { slug } },
        body,
      });
      return requireData(result);
    },

    async listIntegrationOfferings(){return requireData(await client.GET("/integrations"));},
    async listAgents(){return requireData(await client.GET("/agents"));},
    async getAgent(agentId){return requireData(await client.GET("/agents/{agentId}",{params:{path:{agentId}}}));},
    async getAgentTools(agentId){return requireData(await client.GET("/agents/{agentId}/tools",{params:{path:{agentId}}}));},
    async ensureAgentTools(agentId){return requireData(await client.POST("/agents/{agentId}/tools",{params:{path:{agentId}}}));},
    async createAgent(body){return requireData(await client.POST("/agents",{body}));},
    async updateAgentSettings(agentId,defaultModel,defaultReasoningEffort){return requireData(await client.PATCH("/agents/{agentId}/settings",{params:{path:{agentId}},body:{defaultModel,defaultReasoningEffort}}));},
    async inspectAgentModels(agentId){return requireData(await client.POST("/agents/{agentId}/models/inspect",{params:{path:{agentId}}}));},
    async listAgentWorkspaces(agentId){return requireData(await client.GET("/agents/{agentId}/workspaces",{params:{path:{agentId}}}));},
    async bindAgentWorkspace(agentId,workspaceKey,workspaceRoot){const r=await client.PUT("/agents/{agentId}/workspaces/{workspaceKey}",{params:{path:{agentId,workspaceKey}},body:{workspaceRoot}});if(!r.response.ok)throwApiError(r.error,r.response);},
    async unbindAgentWorkspace(agentId,workspaceKey){const r=await client.DELETE("/agents/{agentId}/workspaces/{workspaceKey}",{params:{path:{agentId,workspaceKey}}});if(!r.response.ok)throwApiError(r.error,r.response);},
    async grantAgentWorkspace(agentId,workspaceKey,profileId){const r=await client.PUT("/agents/{agentId}/workspaces/{workspaceKey}/profile-grants/{profileId}",{params:{path:{agentId,workspaceKey,profileId}}});if(!r.response.ok)throwApiError(r.error,r.response);},
    async revokeAgentWorkspace(agentId,workspaceKey,profileId){const r=await client.DELETE("/agents/{agentId}/workspaces/{workspaceKey}/profile-grants/{profileId}",{params:{path:{agentId,workspaceKey,profileId}}});if(!r.response.ok)throwApiError(r.error,r.response);},
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
    async listToolSuites(){return requireData(await client.GET("/tool-suites"));},
    async listWorkspaceToolTargets(providerId){return requireData(await client.GET("/tool-services/{providerId}/workspaces",{params:{path:{providerId}}}));},
    async bindWorkspaceToolTarget(providerId,workspaceKey,sandboxId){const r=await client.PUT("/tool-services/{providerId}/workspaces/{workspaceKey}",{params:{path:{providerId,workspaceKey}},body:{sandboxId}});if(!r.response.ok)throwApiError(r.error,r.response);},
    async unbindWorkspaceToolTarget(providerId,workspaceKey){const r=await client.DELETE("/tool-services/{providerId}/workspaces/{workspaceKey}",{params:{path:{providerId,workspaceKey}}});if(!r.response.ok)throwApiError(r.error,r.response);},
    async getSandboxTools(sandboxId){return requireData(await client.GET("/sandboxes/{sandboxId}/tools",{params:{path:{sandboxId}}}));},
    async ensureSandboxTools(sandboxId){return requireData(await client.POST("/sandboxes/{sandboxId}/tools",{params:{path:{sandboxId}}}));},
    async removeSandboxTools(sandboxId){
      const result=await client.DELETE("/sandboxes/{sandboxId}/tools",{params:{path:{sandboxId}}});
      if(!result.response.ok)throwApiError(result.error,result.response);
    },
    async getSandboxJobTools(sandboxId){return requireData(await client.GET("/sandboxes/{sandboxId}/job-tools",{params:{path:{sandboxId}}}));},
    async getSSHJobRecovery(sandboxId){return requireData(await client.GET("/sandboxes/{sandboxId}/jobs/recovery",{params:{path:{sandboxId}}}));},
    async resolveSSHJobRecovery(sandboxId,taskId,body){return requireData(await client.POST("/sandboxes/{sandboxId}/jobs/recovery/{taskId}/resolve",{params:{path:{sandboxId,taskId}},body}));},
    async getSSHJobAdmission(sandboxId){return requireData(await client.GET("/sandboxes/{sandboxId}/jobs/admission",{params:{path:{sandboxId}}}));},
    async setSSHJobAdmission(sandboxId,body){return requireData(await client.PUT("/sandboxes/{sandboxId}/jobs/admission",{params:{path:{sandboxId}},body}));},
    async ensureSandboxJobTools(sandboxId){return requireData(await client.POST("/sandboxes/{sandboxId}/job-tools",{params:{path:{sandboxId}}}));},
    async removeSandboxJobTools(sandboxId){
      const result=await client.DELETE("/sandboxes/{sandboxId}/job-tools",{params:{path:{sandboxId}}});
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
    async inspectSSHHostKey(host: string, port: number) {
      return requireData(await client.POST("/sandboxes/ssh-remote/host-identity", {body:{host,port}}));
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

    async startOAuthCredential(body: StartOAuthCredentialRequest) {
      const result = await client.POST("/credentials/oauth/flows", { body });
      return requireData(result);
    },

    async getOAuthFlow(flowId: string) {
      const result = await client.GET("/credentials/oauth/flows/{flowId}", {
        params: { path: { flowId } },
      });
      return requireData(result);
    },

    async cancelOAuthFlow(flowId: string) {
      const result = await client.DELETE("/credentials/oauth/flows/{flowId}", {
        params: { path: { flowId } },
      });
      if (!result.response.ok) throwApiError(result.error, result.response);
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

    async disconnectOAuthCredential(namespace: string, name: string) {
      const result = await client.POST("/credentials/{namespace}/{name}/disconnect", {
        params: { path: { namespace, name } },
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
