import { MenderClientInvariantError } from "@mender/sdk";
import type {
  AuditRecord,
  ProviderActivationAuditRecord,
  Credential,
  CredentialStatus,
  InvocationResult,
  MCPServerInstance,
  MenderClient,
  Policy,
  PolicyRule,
  PolicyRuleInput,
  PolicyStatus,
  RuntimeBudget,
  RuntimeGovernanceStatus,
  RuntimeIsolation,
  Profile,
  Provider,
  ProviderMutationResponse,
  ProviderPlugin,
  ProviderPluginInstallation,
  ProviderPluginIssue,
  PluginCatalogueEntry,
  SkillDeployment,
  SkillInspection,
  SkillInstallation,
  SyncResult,
  SystemStatus,
  ToolDetail,
  ToolSummary,
  Tunnel,
  Sandbox,
  Agent,
  AgentTools,
  AgentTask,
  AgentAdmissionEvent,
  SandboxActivity,
  SandboxActivityEvent,
  SandboxProfileGrants,
  SandboxProfileGrantEvents,
  SandboxProfileCapability,
} from "@mender/sdk";

import {
  Service,
  type AuditRecord as BindingAuditRecord,
  type ProviderActivationAuditRecord as BindingProviderActivationAuditRecord,
  type Credential as BindingCredential,
  type CredentialStatus as BindingCredentialStatus,
  type InvocationResult as BindingInvocationResult,
  type MCPServerInstance as BindingMCPServerInstance,
  type Policy as BindingPolicy,
  type PolicyRule as BindingPolicyRule,
  type PolicyRuleInput as BindingPolicyRuleInput,
  type PolicyStatus as BindingPolicyStatus,
  type RuntimeBudget as BindingRuntimeBudget,
  type RuntimeGovernanceStatus as BindingRuntimeGovernanceStatus,
  type RuntimeIsolation as BindingRuntimeIsolation,
  type Profile as BindingProfile,
  type Provider as BindingProvider,
  type ProviderMutationResponse as BindingProviderMutationResponse,
  type ProviderPlugin as BindingProviderPlugin,
  type ProviderPluginInstallation as BindingProviderPluginInstallation,
  type ProviderPluginIssue as BindingProviderPluginIssue,
  type PluginCatalogueEntry as BindingPluginCatalogueEntry,
  type SkillDeployment as BindingSkillDeployment,
  type SkillInspection as BindingSkillInspection,
  type SkillInstallation as BindingSkillInstallation,
  type SkillMetadata as BindingSkillMetadata,
  type SkillResource as BindingSkillResource,
  type SyncResult as BindingSyncResult,
  type SystemStatus as BindingSystemStatus,
  type ToolDetail as BindingToolDetail,
  type ToolSummary as BindingToolSummary,
  type Tunnel as BindingTunnel,
  type Agent as BindingAgent,
  type AgentTools as BindingAgentTools,
  type AgentTask as BindingAgentTask,
  type AgentAdmissionEvent as BindingAgentAdmissionEvent,
  type Sandbox as BindingSandbox,
  type SandboxActivity as BindingSandboxActivity,
  type SandboxProfileGrants as BindingSandboxProfileGrants,
  type SandboxProfileGrantEvents as BindingSandboxProfileGrantEvents,
} from "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding";

export async function installDesktopPlugin(sourcePath:string){
  return providerPluginInstallation(await Service.InstallLocalPlugin(sourcePath));
}

export async function installDesktopProviderPlugin(
  sourcePath: string,
): Promise<ProviderPluginInstallation> {
  return providerPluginInstallation(
    await Service.InstallProviderPlugin(sourcePath),
  );
}

export async function installDesktopSkill(
  sourcePath: string,
): Promise<SkillInstallation> {
  return skillInstallation(await Service.InstallSkill(sourcePath));
}

export async function deployDesktopSkill(
  installationId: string,
  target: SkillDeployment["target"],
  projectRoot = "",
): Promise<SkillDeployment> {
  return skillDeployment(
    await Service.DeploySkill(installationId, target, projectRoot),
  );
}

export function createDesktopMenderClient(): MenderClient {
  return {
    async getSystemStatus() {
      return systemStatus(await Service.GetSystemStatus());
    },

    async listPluginCatalogue(){
      const catalogue=await Service.ListPluginCatalogue();
      return {plugins:(catalogue.plugins??[]).map(pluginCatalogueEntry)};
    },
    async enablePluginInstallation(installationId){
      return {installation:providerPluginInstallation(await Service.EnablePluginInstallation(installationId))};
    },
    async disablePluginInstallation(installationId){
      return {installation:providerPluginInstallation(await Service.DisablePluginInstallation(installationId))};
    },
    async deletePluginInstallation(installationId){
      await Service.DeletePluginInstallation(installationId);
    },
    async listProviderPlugins() {
      const result = await Service.ListProviderPlugins();
      return {
        plugins: (result.plugins ?? []).map(providerPlugin),
        issues: (result.issues ?? []).map(providerPluginIssue),
      };
    },

    async listProviderPluginInstallations() {
      const result = await Service.ListProviderPluginInstallations();
      return {
        installations: (result.installations ?? []).map(
          providerPluginInstallation,
        ),
      };
    },

    async enableProviderPluginInstallation(installationId) {
      return {
        installation: providerPluginInstallation(
          await Service.EnableProviderPluginInstallation(installationId),
        ),
      };
    },

    async disableProviderPluginInstallation(installationId) {
      return {
        installation: providerPluginInstallation(
          await Service.DisableProviderPluginInstallation(installationId),
        ),
      };
    },

    async deleteProviderPluginInstallation(installationId) {
      await Service.DeleteProviderPluginInstallation(installationId);
    },

    async listSkills() {
      const result = await Service.ListSkills();
      return {
        installations: (result.installations ?? []).map(skillInstallation),
      };
    },

    async getSkillInstallation(installationId) {
      return skillInspection(
        await Service.GetSkillInstallation(installationId),
      );
    },

    async trustSkillInstallation(installationId) {
      return {
        installation: skillInstallation(
          await Service.TrustSkillInstallation(installationId),
        ),
      };
    },

    async untrustSkillInstallation(installationId) {
      return {
        installation: skillInstallation(
          await Service.UntrustSkillInstallation(installationId),
        ),
      };
    },

    async enableSkillInstallation(installationId) {
      return {
        installation: skillInstallation(
          await Service.EnableSkillInstallation(installationId),
        ),
      };
    },

    async disableSkillInstallation(installationId) {
      return {
        installation: skillInstallation(
          await Service.DisableSkillInstallation(installationId),
        ),
      };
    },

    async deleteSkillInstallation(installationId) {
      await Service.DeleteSkillInstallation(installationId);
    },

    async listSkillDeployments() {
      const result = await Service.ListSkillDeployments();
      return {
        deployments: (result.deployments ?? []).map(skillDeployment),
      };
    },

    async undeploySkill(deploymentId) {
      await Service.UndeploySkill(deploymentId);
    },

    async listProviders() {
      const result = await Service.ListProviders();
      return { providers: (result.providers ?? []).map(provider) };
    },

    async createProvider(body) {
      return providerMutation(
        await Service.CreateProvider(
          body.namespace,
          body.displayName,
          body.description ?? "",
          body.pluginId,
          body.installationId ?? "",
          body.config,
        ),
      );
    },

    async createHTTPProvider(body) {
      return providerMutation(
        await Service.CreateHTTPProvider(
          body.namespace,
          body.displayName,
          body.description ?? "",
          body.endpoint,
          body.headers ?? {},
          body.credentialHeaders ?? [],
        ),
      );
    },

    async createStdioProvider(body) {
      return providerMutation(
        await Service.CreateStdioProvider(
          body.namespace,
          body.displayName,
          body.description ?? "",
          body.executable,
          body.args ?? [],
          body.directory ?? "",
          body.credentialEnvironment ?? [],
        ),
      );
    },

    async updateProvider(providerId, body) {
      if ("config" in body) {
        return providerMutation(
          await Service.UpdateConfiguredProvider(
            providerId,
            body.displayName,
            body.description ?? "",
            body.config,
          ),
        );
      }
      if (body.transport === "http") {
        return providerMutation(
          await Service.UpdateProvider(
            providerId,
            body.displayName,
            body.description ?? "",
            "http",
            body.endpoint,
            body.headers ?? {},
            body.credentialHeaders ?? [],
            "",
            [],
            "",
            [],
          ),
        );
      }
      return providerMutation(
        await Service.UpdateProvider(
          providerId,
          body.displayName,
          body.description ?? "",
          "stdio",
          "",
          {},
          [],
          body.executable,
          body.args ?? [],
          body.directory ?? "",
          body.credentialEnvironment ?? [],
        ),
      );
    },

    async rebindProvider(providerId, body) {
      return providerMutation(
        await Service.RebindProvider(
          providerId,
          body.installationId,
          body.config,
        ),
      );
    },

    async syncProvider(providerId) {
      return syncResult(await Service.SyncProvider(providerId));
    },

    async deleteProvider(providerId) {
      await Service.DeleteProvider(providerId);
    },

    async browseTools(input = {}) {
      const result = await Service.BrowseTools(
        input.limit ?? 100,
        input.providerId ?? "",
      );
      return { tools: (result.tools ?? []).map(toolSummary) };
    },

    async searchTools(input) {
      const result = await Service.SearchTools(
        input.query,
        input.limit ?? 20,
        input.providerId ?? "",
        input.profile ?? "",
      );
      return { tools: (result.tools ?? []).map(toolSummary) };
    },

    async getTool(toolId) {
      return toolDetail(await Service.GetTool(toolId));
    },

    async callTool(toolId, argumentsValue) {
      return invocationResult(await Service.CallTool(toolId, argumentsValue));
    },

    async listProfiles() {
      const result = await Service.ListProfiles();
      return { profiles: (result.profiles ?? []).map(profile) };
    },

    async createProfile(body) {
      return profile(
        await Service.CreateProfile(body.slug, body.displayName, body.mode),
      );
    },

    async getProfile(slug) {
      return profile(await Service.GetProfile(slug));
    },

    async addProfileTool(slug, body) {
      return profile(
        await Service.AddProfileTool(slug, bindingProfileTool(body)),
      );
    },

    async replaceProfileTools(slug, body) {
      return profile(
        await Service.ReplaceProfileTools(
          slug,
          body.tools.map(bindingProfileTool),
        ),
      );
    },

    async removeProfileTool(slug, toolId) {
      return profile(await Service.RemoveProfileTool(slug, toolId));
    },

    async setProfileTunnel(slug, body) {
      return profile(await Service.SetProfileTunnel(slug, body.tunnelId));
    },

    async listAgents(){
      const result=await Service.ListAgents();
      return {agents:(result.agents??[]).map(agent)};
    },
    async getAgent(agentId){return agent(await Service.GetAgent(agentId));},
    async getAgentTools(agentId){return agentTools(await Service.GetAgentTools(agentId));},
    async ensureAgentTools(agentId){return agentTools(await Service.EnsureAgentTools(agentId));},
    async createAgent(body){return agent(await Service.CreateAgent({
      name:body.name,backend:body.backend,workspaceRoot:body.workspaceRoot,
      isolation:body.isolation??"restricted_host",
    }));},
    async deleteAgent(agentId){await Service.DeleteAgent(agentId);},
    async listAgentTasks(agentId){
      const result=await Service.ListAgentTasks(agentId);
      return {tasks:(result.tasks??[]).map(agentTask)};
    },
    async listAgentAdmissions(agentId){
      const result=await Service.ListAgentAdmissions(agentId);
      return {events:(result.events??[]).map(agentAdmissionEvent)};
    },
    async getAgentTask(taskId){return agentTask(await Service.GetAgentTask(taskId));},
    async cancelAgentTask(taskId){return Service.CancelAgentTask(taskId);},
    async listAgentGrants(agentId){
      const result=await Service.ListAgentGrants(agentId);
      return {profileIds:result.profileIds??[]};
    },
    async grantAgentProfile(agentId,profileId){await Service.GrantAgentProfile(agentId,profileId);},
    async revokeAgentProfile(agentId,profileId){await Service.RevokeAgentProfile(agentId,profileId);},
    async listSandboxes() {
      const result = await Service.ListSandboxes();
      return { sandboxes: (result.sandboxes ?? []).map(sandbox) };
    },

    async getSandbox(sandboxId) {
      return sandbox(await Service.GetSandbox(sandboxId));
    },

    async getSandboxActivity(sandboxId, limit = 50) {
      return sandboxActivity(await Service.GetSandboxActivity(sandboxId, limit));
    },
    async getSandboxProfileGrants(sandboxId) {
      return sandboxProfileGrants(await Service.GetSandboxProfileGrants(sandboxId));
    },
    async putSandboxProfileGrants(sandboxId, profileId, body) {
      return sandboxProfileGrants(await Service.PutSandboxProfileGrants(sandboxId, profileId, body.capabilities));
    },
    async revokeSandboxProfileGrants(sandboxId, profileId) {
      return sandboxProfileGrants(await Service.RevokeSandboxProfileGrants(sandboxId, profileId));
    },
    async getSandboxProfileGrantEvents(sandboxId) {
      return sandboxProfileGrantEvents(await Service.GetSandboxProfileGrantEvents(sandboxId, 100));
    },
    async createLocalSandbox(body) {
      return sandbox(await Service.CreateLocalSandbox(body));
    },

    async getSandboxBackends() {
      return Service.GetSandboxBackends();
    },
    async listDockerImages() {
      const result=await Service.ListDockerImages();
      return {images:(result.images??[]).map(image=>({
        imageDigest:image.imageDigest,displayName:image.displayName,
      }))};
    },
    async createExternalSandbox(body){
      return sandbox(await Service.CreateExternalSandbox({
        name:body.name,installationId:body.installationId,configJson:JSON.stringify(body.config),
      }));
    },
    async reconcileSandbox(sandboxId){
      return sandbox(await Service.ReconcileSandbox(sandboxId));
    },
    async createDaytonaSandbox(body) {
      return sandbox(await Service.CreateDaytonaSandbox(body));
    },
    async createSSHRemoteSandbox(body) {
      return sandbox(await Service.CreateSSHRemoteSandbox(body));
    },
    async reconcileDaytonaSandbox(sandboxId) {
      return sandbox(await Service.ReconcileDaytonaSandbox(sandboxId));
    },
    async createDockerSandbox(body) {
      return sandbox(await Service.CreateDockerSandbox(body));
    },
    async reconcileDockerSandbox(sandboxId) {
      return sandbox(await Service.ReconcileDockerSandbox(sandboxId));
    },

    async startSandbox(sandboxId) {
      return sandbox(await Service.StartSandbox(sandboxId));
    },
    async stopSandbox(sandboxId) {
      return sandbox(await Service.StopSandbox(sandboxId));
    },
    async destroySandbox(sandboxId) {
      return sandbox(await Service.DestroySandbox(sandboxId));
    },

    async listTunnels() {
      const result = await Service.ListTunnels();
      return { tunnels: (result.tunnels ?? []).map(tunnel) };
    },

    async createExternalTunnel(body){
      return tunnel(await Service.CreateExternalTunnel(
        body.displayName,body.installationId,body.externalTunnelId,body.runtimeCredentialRef,
      ));
    },
    async createOpenAITunnel(body) {
      return tunnel(
        await Service.CreateOpenAITunnel(
          body.displayName,
          body.externalTunnelId,
          body.runtimeCredentialRef,
        ),
      );
    },

    async deleteTunnel(tunnelId) {
      await Service.DeleteTunnel(tunnelId);
    },

    async listMCPServers() {
      const result = await Service.ListMCPServers();
      return { servers: (result.servers ?? []).map(mcpServerInstance) };
    },

    async startMCPServer(body) {
      return mcpServerInstance(
        await Service.StartMCPServer(body.profile, body.address),
      );
    },

    async restartMCPServer(serverId) {
      return mcpServerInstance(await Service.RestartMCPServer(serverId));
    },

    async stopMCPServer(serverId) {
      await Service.StopMCPServer(serverId);
    },

    async listAudit(limit = 50) {
      const result = await Service.ListAudit(limit);
      return { records: (result.records ?? []).map(auditRecord) };
    },

    async listProviderActivationAudit(limit = 50) {
      const result = await Service.ListProviderActivationAudit(limit);
      return {
        records: (result.records ?? []).map(providerActivationAuditRecord),
      };
    },

    async getRuntimeGovernance() {
      return runtimeGovernanceStatus(await Service.GetRuntimeGovernance());
    },

    async updateRuntimeBudget(body) {
      return runtimeGovernanceStatus(
        await Service.UpdateRuntimeBudget(bindingRuntimeBudget(body)),
      );
    },

    async updateRuntimeIsolation(body) {
      return runtimeGovernanceStatus(
        await Service.UpdateRuntimeIsolation(bindingRuntimeIsolation(body)),
      );
    },

    async getPolicyStatus() {
      return policyStatus(await Service.GetPolicyStatus());
    },

    async getPolicy() {
      return policy(await Service.GetPolicy());
    },

    async setPolicyDefault(body) {
      return policy(await Service.SetPolicyDefault(body.decision));
    },

    async createPolicyRule(body) {
      return policyRule(
        await Service.CreatePolicyRule(bindingPolicyRuleInput(body)),
      );
    },

    async updatePolicyRule(ruleId, body) {
      return policyRule(
        await Service.UpdatePolicyRule(ruleId, bindingPolicyRuleInput(body)),
      );
    },

    async deletePolicyRule(ruleId) {
      await Service.DeletePolicyRule(ruleId);
    },

    async setPolicyRuleEnabled(ruleId, enabled) {
      return policyRule(await Service.SetPolicyRuleEnabled(ruleId, enabled));
    },

    async getCredentialStatus() {
      return credentialStatus(await Service.GetCredentialStatus());
    },

    async listCredentials() {
      const result = await Service.ListCredentials();
      return { credentials: (result.credentials ?? []).map(credential) };
    },

    async getCredential(namespace, name) {
      return credential(await Service.GetCredential(namespace, name));
    },

    async createNativeCredential(body) {
      return credential(
        await Service.CreateNativeCredential(
          body.namespace,
          body.name,
          body.displayName,
          body.secret,
        ),
      );
    },

    async createEnvironmentCredential(body) {
      return credential(
        await Service.CreateEnvironmentCredential(
          body.namespace,
          body.name,
          body.displayName,
          body.environmentVariable,
        ),
      );
    },

    async rotateCredential(namespace, name, body) {
      return credential(
        await Service.RotateNativeCredential(namespace, name, body.secret),
      );
    },

    async deleteCredential(namespace, name) {
      await Service.DeleteCredential(namespace, name);
    },
  };
}

function systemStatus(value: BindingSystemStatus): SystemStatus {
  return {
    ready: value.ready,
    policy: policyStatus(value.policy),
    credentials: credentialStatus(value.credentials),
    mcp: {
      stdio: value.mcp.stdio,
      streamableHttp: value.mcp.streamableHttp,
      exposureModes: (value.mcp.exposureModes ?? []).map(exposureMode),
    },
  };
}

function runtimeGovernanceStatus(
  value: BindingRuntimeGovernanceStatus,
): RuntimeGovernanceStatus {
  return {
    budget: {
      maxInvocations: value.budget.maxInvocations,
      maxPrincipalInvocations: value.budget.maxPrincipalInvocations,
      maxProviderInvocations: value.budget.maxProviderInvocations,
      maxToolInvocations: value.budget.maxToolInvocations,
      maxProviderActivations: value.budget.maxProviderActivations,
      maxExecutionTimeoutMs: value.budget.maxExecutionTimeoutMs,
      maxArgumentBytes: value.budget.maxArgumentBytes,
      maxResultBytes: value.budget.maxResultBytes,
    },
    isolation: {
      workspaceShell: isolationMode(value.isolation.workspaceShell),
      externalProvider: isolationMode(value.isolation.externalProvider),
      tunnelClient: isolationMode(value.isolation.tunnelClient),
    },
    backend: {
      name: value.backend.name,
      restrictedHostAvailable: value.backend.restrictedHostAvailable,
    },
    usage: {
      invocations: value.usage.invocations,
      activations: value.usage.activations,
    },
  };
}

function bindingRuntimeBudget(value: RuntimeBudget): BindingRuntimeBudget {
  return { ...value };
}

function bindingRuntimeIsolation(value: RuntimeIsolation): BindingRuntimeIsolation {
  return { ...value };
}

function isolationMode(value: string): "host" | "restricted_host" {
  if (value === "host" || value === "restricted_host") {
    return value;
  }
  throw new MenderClientInvariantError(`Unsupported desktop isolation mode: ${value}`);
}

function policyStatus(value: BindingPolicyStatus): PolicyStatus {
  if (value.mode !== "configured") {
    throw new MenderClientInvariantError(`Unsupported desktop policy mode: ${value.mode}`);
  }
  if (value.approval !== "fail_closed_when_required") {
    throw new MenderClientInvariantError(`Unsupported desktop approval mode: ${value.approval}`);
  }
  if (!value.editable) {
    throw new MenderClientInvariantError("Desktop policy unexpectedly reported read-only state");
  }
  if (value.ruleCount < 0 || value.enabledRuleCount < 0) {
    throw new MenderClientInvariantError("Desktop policy reported invalid Rule counts");
  }
  return {
    mode: "configured",
    editable: true,
    approval: value.approval,
    defaultDecision: policyDecision(value.defaultDecision),
    ruleCount: value.ruleCount,
    enabledRuleCount: value.enabledRuleCount,
  };
}

function policy(value: BindingPolicy): Policy {
  return {
    defaultDecision: policyDecision(value.defaultDecision),
    rules: (value.rules ?? []).map(policyRule),
  };
}

function policyRule(value: BindingPolicyRule): PolicyRule {
  return {
    ruleId: value.ruleId,
    displayName: value.displayName,
    enabled: value.enabled,
    decision: policyDecision(value.decision),
    match: {
      actions: (value.match.actions ?? []).map(policyAction),
      origins: (value.match.origins ?? []).map(policyOrigin),
      principalKinds: (value.match.principalKinds ?? []).map(policyPrincipalKind),
      principalRefs: value.match.principalRefs ?? [],
      scopeKinds: (value.match.scopeKinds ?? []).map(policyScopeKind),
      profileRefs: value.match.profileRefs ?? [],
      providerIds: value.match.providerIds ?? [],
      toolIds: value.match.toolIds ?? [],
      factKnowledge: policyFactKnowledge(value.match.factKnowledge),
      effectsAny: value.match.effectsAny ?? [],
      resourcesAny: (value.match.resourcesAny ?? []).map((resource) => ({
        kind: resource.kind,
        operator: policyResourceOperator(resource.operator),
        value: resource.value,
      })),
    },
    createdAt: value.createdAt,
    updatedAt: value.updatedAt,
  };
}

function bindingPolicyRuleInput(
  value: PolicyRuleInput,
): BindingPolicyRuleInput {
  return {
    displayName: value.displayName,
    enabled: value.enabled,
    decision: value.decision,
    match: {
      actions: value.match.actions,
      origins: value.match.origins,
      principalKinds: value.match.principalKinds,
      principalRefs: value.match.principalRefs,
      scopeKinds: value.match.scopeKinds,
      profileRefs: value.match.profileRefs,
      providerIds: value.match.providerIds,
      toolIds: value.match.toolIds,
      factKnowledge: value.match.factKnowledge,
      effectsAny: value.match.effectsAny,
      resourcesAny: value.match.resourcesAny.map((resource) => ({
        kind: resource.kind,
        operator: resource.operator,
        value: resource.value,
      })),
    },
  };
}

function policyDecision(value: string): Policy["defaultDecision"] {
  if (
    value === "allow" ||
    value === "require_approval" ||
    value === "deny"
  ) {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported desktop Policy decision: " + value);
}

function policyAction(
  value: string,
): PolicyRule["match"]["actions"][number] {
  if (value === "tool.invoke" || value === "provider.activate" ||
      value === "sandbox.acquire" || value === "sandbox.provision" ||
      value === "sandbox.start" || value === "sandbox.stop" ||
      value === "sandbox.destroy") {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported desktop Policy action: " + value);
}

function policyPrincipalKind(
  value: string,
): PolicyRule["match"]["principalKinds"][number] {
  if (value === "local_user" || value === "service") {
    return value;
  }
  throw new MenderClientInvariantError(
    "Unsupported desktop Policy Principal kind: " + value,
  );
}

function policyOrigin(
  value: string,
): PolicyRule["match"]["origins"][number] {
  if (
    value === "cli" ||
    value === "mcp" ||
    value === "management_http" ||
    value === "desktop" ||
    value === "internal"
  ) {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported desktop Policy origin: " + value);
}

function policyScopeKind(
  value: string,
): PolicyRule["match"]["scopeKinds"][number] {
  if (value === "catalog" || value === "profile") {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported desktop Policy scope kind: " + value);
}

function policyFactKnowledge(
  value: string,
): PolicyRule["match"]["factKnowledge"] {
  if (value === "any" || value === "known" || value === "unknown") {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported desktop Policy fact knowledge: " + value);
}

function policyResourceOperator(
  value: string,
): PolicyRule["match"]["resourcesAny"][number]["operator"] {
  if (value === "exact" || value === "prefix") {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported desktop Policy resource operator: " + value);
}

function agent(value:BindingAgent):Agent {
  if(value.backend!=="codex"&&value.backend!=="grok"){
    throw new MenderClientInvariantError("Unknown Agent Backend: "+value.backend);
  }
  if(value.isolation!=="host"&&value.isolation!=="restricted_host"){
    throw new MenderClientInvariantError("Unknown Agent isolation: "+value.isolation);
  }
  return {...value,backend:value.backend,isolation:value.isolation};
}
function agentTools(value:BindingAgentTools):AgentTools {
  if(value.suiteId!=="agent.delegation") {
    throw new MenderClientInvariantError("Unknown Agent tool suite: "+value.suiteId);
  }
  if(value.status!=="missing"&&value.status!=="ready"&&value.status!=="incomplete"&&
    value.status!=="legacy"&&value.status!=="conflict"){
    throw new MenderClientInvariantError("Unknown Agent tools state: "+value.status);
  }
  return {
    ...value,
    suiteId:"agent.delegation",
    status:value.status,
    tools:(value.tools??[]).map(item=>({toolId:item.toolId,sourceKey:item.sourceKey,name:item.name,title:item.title})),
  };
}
function agentAdmissionEvent(value:BindingAgentAdmissionEvent):AgentAdmissionEvent{
  if(value.action!=="agent.activate" && value.action!=="agent.task.submit" && value.action!=="agent.task.cancel"){
    throw new MenderClientInvariantError("Unknown Agent action: "+value.action);
  }
  if(value.principalKind!=="local_user"&&value.principalKind!=="service"){
    throw new MenderClientInvariantError("Unknown Agent Principal: "+value.principalKind);
  }
  if(value.origin!=="cli"&&value.origin!=="mcp"&&value.origin!=="management_http"&&
    value.origin!=="desktop"&&value.origin!=="internal"){
    throw new MenderClientInvariantError("Unknown Agent origin: "+value.origin);
  }
  if(value.scopeKind!=="catalog"&&value.scopeKind!=="profile"){
    throw new MenderClientInvariantError("Unknown Agent scope: "+value.scopeKind);
  }
  if(value.policyKind!=="allow"&&value.policyKind!=="deny"&&
    value.policyKind!=="require_approval"&&value.policyKind!=="failed"){
    throw new MenderClientInvariantError("Unknown Agent Policy decision: "+value.policyKind);
  }
  if(value.status!=="admitted"&&value.status!=="denied"&&value.status!=="failed"){
    throw new MenderClientInvariantError("Unknown Agent audit status: "+value.status);
  }
  return {...value,action:value.action,principalKind:value.principalKind,origin:value.origin,
    scopeKind:value.scopeKind,policyKind:value.policyKind,status:value.status};
}
function agentTask(value:BindingAgentTask):AgentTask {
  switch(value.status){
    case "queued":case "running":case "completed":case "failed":
    case "cancelled":case "unknown":
      return {...value,status:value.status};
    default:
      throw new MenderClientInvariantError("Unknown Agent Task status: "+value.status);
  }
}

function credentialStatus(value: BindingCredentialStatus): CredentialStatus {
  if (value.resolver !== "inventory") {
    throw new MenderClientInvariantError(
      `Unsupported desktop credential resolver: ${value.resolver}`,
    );
  }
  if (!value.persistentStore || !value.editable) {
    throw new MenderClientInvariantError(
      "Desktop credential inventory unexpectedly reported disabled persistence or editing",
    );
  }
  return {
    resolver: "inventory",
    persistentStore: true,
    editable: true,
  };
}

function credential(value: BindingCredential): Credential {
  if (value.source !== "native" && value.source !== "environment") {
    throw new MenderClientInvariantError(`Unsupported desktop Credential source: ${value.source}`);
  }
  return {
    reference: value.reference,
    namespace: value.namespace,
    name: value.name,
    displayName: value.displayName,
    source: value.source,
    environmentVariable: value.environmentVariable,
    createdAt: value.createdAt,
    updatedAt: value.updatedAt,
  };
}

function providerPlugin(value: BindingProviderPlugin): ProviderPlugin {
  return {
    pluginId: value.pluginId,
    version: value.version,
    packageVersion: value.packageVersion,
    displayName: value.displayName,
    description: value.description,
    configVersion: value.configVersion,
    sourceKind: providerPluginSourceKind(value.sourceKind),
    ...(value.installationId === undefined
      ? {}
      : { installationId: value.installationId }),
    ...(value.protocolVersion === undefined
      ? {}
      : { protocolVersion: value.protocolVersion }),
    ...(value.configSchema === undefined
      ? {}
      : { configSchema: providerPluginConfigSchema(value.configSchema) }),
    ...(value.uiSchema === undefined
      ? {}
      : { uiSchema: providerPluginUISchema(value.uiSchema) }),
    capabilities: value.capabilities ?? [],
    enabled: value.enabled,
  };
}

function providerPluginIssue(
  value: BindingProviderPluginIssue,
): ProviderPluginIssue {
  return {
    installationId: value.installationId,
    code: value.code,
    message: value.message,
  };
}

function providerPluginSourceKind(
  value: string,
): ProviderPlugin["sourceKind"] {
  if (value === "builtin" || value === "external") {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported Provider plugin source kind: " + value);
}

function providerPluginConfigSchema(
  value: unknown,
): NonNullable<ProviderPlugin["configSchema"]> {
  if (typeof value === "boolean") {
    return value;
  }
  if (typeof value === "object" && value !== null && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  throw new MenderClientInvariantError("Unsupported Provider plugin config schema");
}

function providerPluginUISchema(
  value: unknown,
): NonNullable<ProviderPlugin["uiSchema"]> {
  if (typeof value === "object" && value !== null && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  throw new MenderClientInvariantError("Unsupported Provider plugin UI schema");
}

function pluginCatalogueEntry(value:BindingPluginCatalogueEntry):PluginCatalogueEntry {
  if(!["provider.tools","sandbox.backend","tunnel.transport","agent.backend"].includes(value.kind) ||
     !["builtin","local","planned"].includes(value.sourceKind) ||
     !["enabled","disabled","invalid","unavailable","planned"].includes(value.status)){
    throw new MenderClientInvariantError("Unsupported typed plugin catalogue entry");
  }
  // A planned Marketplace preview is inert metadata, never an executable
  // built-in or a forged installed package. Fail closed on mismatched shapes.
  const planned=value.sourceKind==="planned";
  if(planned!==(value.status==="planned") ||
     (planned && (value.enabled || Boolean(value.installationId) ||
       Boolean(value.digest) || value.packageVersion!=="" ||
       (value.capabilities?.length??0)!==0))){
    throw new MenderClientInvariantError("Unsupported planned plugin catalogue state");
  }
  const schema:unknown=value.configSchema;
  if(schema!==undefined && typeof schema!=="boolean" &&
     (typeof schema!=="object" || schema===null || Array.isArray(schema))){
    throw new MenderClientInvariantError("Unsupported plugin configuration schema");
  }
  return {
    key:value.key,pluginId:value.pluginId,extensionId:value.extensionId,
    kind:value.kind as PluginCatalogueEntry["kind"],displayName:value.displayName,
    description:value.description,packageVersion:value.packageVersion,
    sourceKind:value.sourceKind as PluginCatalogueEntry["sourceKind"],
    status:value.status as PluginCatalogueEntry["status"],
    enabled:value.enabled,capabilities:value.capabilities??[],usageCount:value.usageCount,
    ...(value.installationId?{installationId:value.installationId}:{}),
    ...(value.digest?{digest:value.digest}:{}),
    ...(value.installedAt?{installedAt:value.installedAt}:{}),
    ...(value.configVersion?{configVersion:value.configVersion}:{}),
    ...(value.protocolVersion?{protocolVersion:value.protocolVersion}:{}),
    ...(schema!==undefined?{configSchema:schema as NonNullable<PluginCatalogueEntry["configSchema"]>}:{}),
    ...(value.issue?{issue:value.issue}:{}),
  };
}

function providerPluginInstallation(
  value: BindingProviderPluginInstallation,
): ProviderPluginInstallation {
  return {
    installationId: value.installationId,
    pluginId: value.pluginId,
    packageVersion: value.packageVersion,
    sourceKind: "local",
    digest: value.digest,
    status: providerPluginInstallationStatus(value.status),
    installedAt: value.installedAt,
    ...(value.displayName === undefined
      ? {}
      : { displayName: value.displayName }),
    ...(value.description === undefined
      ? {}
      : { description: value.description }),
    ...(value.configVersion === undefined
      ? {}
      : { configVersion: value.configVersion }),
    ...(value.protocolVersion === undefined
      ? {}
      : { protocolVersion: value.protocolVersion }),
    ...(Array.isArray(value.capabilities)
      ? { capabilities: value.capabilities }
      : {}),
  };
}

function providerPluginInstallationStatus(
  value: string,
): ProviderPluginInstallation["status"] {
  if (value === "enabled" || value === "disabled" || value === "invalid") {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported Provider plugin installation status: " + value);
}

function skillInstallation(
  value: BindingSkillInstallation,
): SkillInstallation {
  return {
    skillId: value.skillId,
    installationId: value.installationId,
    name: value.name,
    description: value.description,
    sourceKind: skillSourceKind(value.sourceKind),
    sourceReference: value.sourceReference,
    digest: value.digest,
    versionHint: value.versionHint,
    trusted: value.trusted,
    enabled: value.enabled,
    builtIn: value.builtIn,
    ...(value.installedAt === undefined
      ? {}
      : { installedAt: value.installedAt }),
  };
}

function skillSourceKind(
  value: string,
): SkillInstallation["sourceKind"] {
  if (value === "builtin" || value === "local-directory") {
    return value;
  }
  throw new MenderClientInvariantError(
    "Unsupported Skill source kind: " + value,
  );
}

function skillInspection(value: BindingSkillInspection): SkillInspection {
  return {
    installation: skillInstallation(value.installation),
    metadata: skillMetadata(value.metadata),
    resources: (value.resources ?? []).map(skillResource),
    totalBytes: value.totalBytes,
  };
}

function skillMetadata(
  value: BindingSkillMetadata,
): SkillInspection["metadata"] {
  return {
    name: value.name,
    description: value.description,
    ...(value.license === undefined ? {} : { license: value.license }),
    ...(value.compatibility === undefined
      ? {}
      : { compatibility: value.compatibility }),
    metadata: Object.fromEntries(
      Object.entries(value.metadata ?? {}).filter(
        (entry): entry is [string, string] => typeof entry[1] === "string",
      ),
    ),
    ...(value.allowedTools === undefined
      ? {}
      : { allowedTools: value.allowedTools }),
  };
}

function skillResource(
  value: BindingSkillResource,
): SkillInspection["resources"][number] {
  return {
    path: value.path,
    kind: skillResourceKind(value.kind),
    size: value.size,
    sha256: value.sha256,
    executable: value.executable,
  };
}

function skillResourceKind(
  value: string,
): SkillInspection["resources"][number]["kind"] {
  if (
    value === "manifest" ||
    value === "script" ||
    value === "reference" ||
    value === "asset" ||
    value === "other"
  ) {
    return value;
  }
  throw new MenderClientInvariantError(
    "Unsupported Skill resource kind: " + value,
  );
}

function skillDeployment(
  value: BindingSkillDeployment,
): SkillDeployment {
  return {
    deploymentId: value.deploymentId,
    installationId: value.installationId,
    target: skillDeploymentTarget(value.target),
    root: value.root,
    destination: value.destination,
    digest: value.digest,
    deployedAt: value.deployedAt,
    drift: skillDeploymentDrift(value.drift),
    ...(value.currentDigest === undefined
      ? {}
      : { currentDigest: value.currentDigest }),
  };
}

function skillDeploymentTarget(
  value: string,
): SkillDeployment["target"] {
  if (
    value === "agents-project" ||
    value === "github-project" ||
    value === "claude-project" ||
    value === "agents-user" ||
    value === "copilot-user"
  ) {
    return value;
  }
  throw new MenderClientInvariantError(
    "Unsupported Skill deployment target: " + value,
  );
}

function skillDeploymentDrift(
  value: string,
): SkillDeployment["drift"] {
  if (value === "current" || value === "drifted" || value === "missing") {
    return value;
  }
  throw new MenderClientInvariantError(
    "Unsupported Skill deployment drift: " + value,
  );
}

function provider(value: BindingProvider): Provider {
  const transport = providerTransport(value.transport);
  return {
    providerId: value.providerId,
    namespace: value.namespace,
    displayName: value.displayName,
    description: value.description,
    createdAt: value.createdAt,
    updatedAt: value.updatedAt,
    ...(value.pluginId === undefined ? {} : { pluginId: value.pluginId }),
    ...(value.installationId === undefined
      ? {}
      : { installationId: value.installationId }),
    ...(value.pluginVersion === undefined
      ? {}
      : { pluginVersion: value.pluginVersion }),
    ...(value.pluginDisplayName === undefined
      ? {}
      : { pluginDisplayName: value.pluginDisplayName }),
    ...(value.configVersion === undefined
      ? {}
      : { configVersion: value.configVersion }),
    ...(value.connectionSummary === undefined
      ? {}
      : { connectionSummary: value.connectionSummary }),
    ...(value.config === undefined || value.config === null
      ? {}
      : { config: value.config }),
    transport,
    toolCount: value.toolCount,
    ...(value.endpoint === undefined ? {} : { endpoint: value.endpoint }),
    ...(value.headers === undefined || value.headers === null
      ? {}
      : {
          headers: Object.fromEntries(
            Object.entries(value.headers).filter(
              (entry): entry is [string, string] => entry[1] !== undefined,
            ),
          ),
        }),
    ...(value.executable === undefined
      ? {}
      : { executable: value.executable }),
    ...(value.args === undefined || value.args === null
      ? {}
      : { args: value.args }),
    ...(value.directory === undefined ? {} : { directory: value.directory }),
    ...(value.credentialHeaders === undefined || value.credentialHeaders === null
      ? {}
      : { credentialHeaders: value.credentialHeaders }),
    ...(value.credentialEnvironment === undefined ||
    value.credentialEnvironment === null
      ? {}
      : { credentialEnvironment: value.credentialEnvironment }),
  };
}

function providerMutation(
  value: BindingProviderMutationResponse,
): ProviderMutationResponse {
  return {
    provider: provider(value.provider),
    sync: syncResult(value.sync),
  };
}

function syncResult(value: BindingSyncResult): SyncResult {
  return {
    created: value.created,
    updated: value.updated,
    removed: value.removed,
    total: value.total,
  };
}

function toolSummary(value: BindingToolSummary): ToolSummary {
  return {
    rank: value.rank,
    toolId: value.toolId,
    providerId: value.providerId,
    name: value.name,
    title: value.title,
    description: value.description,
  };
}

function toolDetail(value: BindingToolDetail): ToolDetail {
  return {
    toolId: value.toolId,
    providerId: value.providerId,
    sourceKey: value.sourceKey,
    name: value.name,
    title: value.title,
    description: value.description,
    inputSchema: value.inputSchema,
    ...(value.outputSchema === undefined
      ? {}
      : { outputSchema: value.outputSchema }),
    tags: value.tags ?? [],
  };
}

function invocationResult(value: BindingInvocationResult): InvocationResult {
  const kind = invocationKind(value.kind);
  return {
    invocationId: value.invocationId,
    kind,
    ...(value.content === undefined ? {} : { content: value.content }),
    ...(value.requestId === undefined ? {} : { requestId: value.requestId }),
    ...(value.message === undefined ? {} : { message: value.message }),
    ...(value.inputSchema === undefined
      ? {}
      : { inputSchema: value.inputSchema }),
    ...(value.taskId === undefined ? {} : { taskId: value.taskId }),
    ...(value.status === undefined ? {} : { status: value.status }),
    ...(value.source === undefined
      ? {}
      : { source: invocationFailureSource(value.source) }),
    ...(value.code === undefined ? {} : { code: value.code }),
  };
}

function profile(value: BindingProfile): Profile {
  const mode = exposureMode(value.mode);
  return {
    profileId: value.profileId,
    slug: value.slug,
    displayName: value.displayName,
    mode,
    ...(value.tunnelId === undefined ? {} : { tunnelId: value.tunnelId }),
    tools: (value.tools ?? []).map((tool) => ({
      toolId: tool.toolId,
      direct: tool.direct,
      alias: tool.alias,
      titleOverride: tool.titleOverride,
      descriptionOverride: tool.descriptionOverride,
      policyConstraint: profilePolicyConstraint(tool.policyConstraint),
      concurrencyConstraint: profileConcurrencyConstraint(
        tool.concurrencyConstraint,
      ),
      credentialRefs: tool.credentialRefs ?? [],
    })),
  };
}

function bindingProfileTool(tool: {
  toolId: string;
  direct?: boolean;
  alias?: string;
  titleOverride?: string;
  descriptionOverride?: string;
  policyConstraint?: "inherit" | "require_approval" | "deny";
  concurrencyConstraint?: "inherit" | "serial";
  credentialRefs?: string[];
}) {
  return {
    toolId: tool.toolId,
    direct: tool.direct ?? false,
    alias: tool.alias ?? "",
    titleOverride: tool.titleOverride ?? "",
    descriptionOverride: tool.descriptionOverride ?? "",
    policyConstraint: tool.policyConstraint ?? "inherit",
    concurrencyConstraint: tool.concurrencyConstraint ?? "inherit",
    credentialRefs: tool.credentialRefs ?? [],
  };
}

function profilePolicyConstraint(
  value: string,
): "inherit" | "require_approval" | "deny" {
  if (
    value === "inherit" ||
    value === "require_approval" ||
    value === "deny"
  ) {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported desktop Profile policy constraint: " + value);
}

function profileConcurrencyConstraint(
  value: string,
): "inherit" | "serial" {
  if (value === "inherit" || value === "serial") {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported desktop Profile concurrency constraint: " + value);
}

function mcpServerInstance(
  value: BindingMCPServerInstance,
): MCPServerInstance {
  if (value.transport !== "streamable-http") {
    throw new MenderClientInvariantError(
      `Unsupported managed MCP server transport: ${value.transport}`,
    );
  }
  if (value.status !== "running") {
    throw new MenderClientInvariantError(`Unsupported managed MCP server status: ${value.status}`);
  }
  return {
    serverId: value.serverId,
    profile: value.profile,
    transport: "streamable-http",
    address: value.address,
    endpoint: value.endpoint,
    status: "running",
    startedAt: value.startedAt,
    ...(value.tunnelId === undefined ? {} : { tunnelId: value.tunnelId }),
    ...(value.tunnelDisplayName === undefined
      ? {}
      : { tunnelDisplayName: value.tunnelDisplayName }),
    ...(value.tunnelProvider === undefined
      ? {}
      : { tunnelProvider: tunnelProvider(value.tunnelProvider) }),
    ...(value.tunnelExternalId === undefined
      ? {}
      : { tunnelExternalId: value.tunnelExternalId }),
    ...(value.tunnelStatus === undefined
      ? {}
      : { tunnelStatus: tunnelStatus(value.tunnelStatus) }),
  };
}

const SANDBOX_PROFILE_CAPABILITIES: ReadonlySet<string> = new Set([
  "file.read", "file.search", "file.edit", "process.shell",
]);

function sandboxProfileCapability(value: string): SandboxProfileCapability {
  if (!SANDBOX_PROFILE_CAPABILITIES.has(value)) {
    throw new MenderClientInvariantError("Unsupported Sandbox Profile capability");
  }
  return value as SandboxProfileCapability;
}

function sandboxProfileGrants(value: BindingSandboxProfileGrants): SandboxProfileGrants {
  return {
    sandboxId: value.sandboxId,

    grants: (value.grants ?? []).map((grant) => ({
      profileId: grant.profileId,
      capabilities: (grant.capabilities ?? []).map(sandboxProfileCapability),
    })),
  };
}

function sandboxProfileGrantEvents(value: BindingSandboxProfileGrantEvents): SandboxProfileGrantEvents {
  return {
    sandboxId: value.sandboxId,

    events: (value.events ?? []).map((event) => {
      if (event.action !== "grant" && event.action !== "revoke") {
        throw new MenderClientInvariantError("Unsupported Sandbox grant audit action");
      }
      return {
        profileId: event.profileId,
        capability: sandboxProfileCapability(event.capability),
        action: event.action,
        occurredAt: event.occurredAt,
      };
    }),
  };
}

function sandboxActivity(value: BindingSandboxActivity): SandboxActivity {
  return {
    sandboxId:value.sandboxId,
    limit:value.limit,
    events:(value.events ?? []).map((event):SandboxActivityEvent=>({
      kind:event.kind as SandboxActivityEvent["kind"],
      action:event.action as SandboxActivityEvent["action"],
      status:event.status as SandboxActivityEvent["status"],
      occurredAt:event.occurredAt,
      principalKind:event.principalKind,
      origin:event.origin,
      ...(event.policyDecision?{policyDecision:event.policyDecision}:{}),
      ...(event.generation!==undefined && event.generation>0?{generation:event.generation}:{}),
      ...(event.previousState?{previousState:event.previousState}:{}),
      ...(event.nextState?{nextState:event.nextState}:{}),
    })),
  };
}

function sandbox(value: BindingSandbox): Sandbox {
  if (!["local_workspace", "docker", "daytona", "ssh_remote", "e2b", "external"].includes(value.backend) ||
      !["host", "restricted_host", "container", "remote"].includes(value.isolation) ||
      !["defined", "provisioning", "ready", "stopping", "stopped", "destroying", "destroyed", "failed", "unknown", "orphaned"].includes(value.state) ||
      (value.ownerKind !== "local_user" && value.ownerKind !== "service")) {
    throw new MenderClientInvariantError("Unsupported desktop Sandbox metadata");
  }
  return {
    sandboxId: value.sandboxId,
    ...(value.pluginId?{pluginId:value.pluginId}:{}),
    ...(value.extensionId?{extensionId:value.extensionId}:{}),
    ...(value.installationId?{installationId:value.installationId}:{}),
    name: value.name,
    backend: value.backend as Sandbox["backend"],
    isolation: value.isolation as Sandbox["isolation"],
    state: value.state as Sandbox["state"],
    localRoot: value.localRoot,
    ownerKind: value.ownerKind,
    ownerRef: value.ownerRef,
    activeLeases: value.activeLeases,
    backendAvailable: value.backendAvailable,
  };
}

function tunnel(value: BindingTunnel): Tunnel {
  return {
    tunnelId: value.tunnelId,
    ...(value.pluginId?{pluginId:value.pluginId}:{}),
    ...(value.extensionId?{extensionId:value.extensionId}:{}),
    ...(value.installationId?{installationId:value.installationId}:{}),
    displayName: value.displayName,
    provider: tunnelProvider(value.provider),
    externalTunnelId: value.externalTunnelId,
    runtimeCredentialRef: value.runtimeCredentialRef,
  };
}

function tunnelProvider(value: string): Tunnel["provider"] {
  if (value === "openai" || value === "external") {
    return value;
  }
  throw new MenderClientInvariantError("Unsupported Tunnel provider: " + value);
}

function tunnelStatus(value: string): NonNullable<MCPServerInstance["tunnelStatus"]> {
  if (["starting","ready","degraded","failed","exited","running"].includes(value)) {
    return value as NonNullable<MCPServerInstance["tunnelStatus"]>;
  }
  throw new MenderClientInvariantError("Unsupported Tunnel status: " + value);
}

function auditRecord(value: BindingAuditRecord): AuditRecord {
  return {
    invocationId: value.invocationId,
    toolId: value.toolId,
    ...(value.providerId === undefined
      ? {}
      : { providerId: value.providerId }),
    principalKind: auditPrincipalKind(value.principalKind),
    principalRef: value.principalRef,
    startedAt: value.startedAt,
    finishedAt: value.finishedAt,
    policyKind: value.policyKind,
    policyReason: value.policyReason,
    approval: value.approval,
    status: value.status,
    resultKind: value.resultKind,
    errorClass: value.errorClass,
    errorSummary: value.errorSummary,
    origin: value.origin,
    scopeKind: value.scopeKind,
    scopeReference: value.scopeReference,
    outcomeKind: value.outcomeKind,
    rejectionCode: value.rejectionCode,
    failureSource: value.failureSource,
    failureCode: value.failureCode,
  };
}

function providerActivationAuditRecord(
  value: BindingProviderActivationAuditRecord,
): ProviderActivationAuditRecord {
  if (value.action !== "provider.activate") {
    throw new MenderClientInvariantError(
      `Unsupported Provider activation Audit action: ${value.action}`,
    );
  }
  if (
    value.status !== "completed" &&
    value.status !== "denied" &&
    value.status !== "failed"
  ) {
    throw new MenderClientInvariantError(
      `Unsupported Provider activation Audit status: ${value.status}`,
    );
  }
  return {
    activationId: value.activationId,
    providerId: value.providerId,
    action: "provider.activate",
    principalKind: livePrincipalKind(value.principalKind),
    principalRef: value.principalRef,
    origin: policyOrigin(value.origin),
    scopeKind: policyScopeKind(value.scopeKind),
    scopeReference: value.scopeReference,
    startedAt: value.startedAt,
    finishedAt: value.finishedAt,
    policyKind: value.policyKind,
    policyReason: value.policyReason,
    approval: value.approval,
    status: value.status,
    errorClass: value.errorClass,
    errorSummary: value.errorSummary,
  };
}

function livePrincipalKind(value: string): "local_user" | "service" {
  if (value === "local_user" || value === "service") {
    return value;
  }
  throw new MenderClientInvariantError(
    `Unsupported live Principal kind: ${value}`,
  );
}

function auditPrincipalKind(
  value: string,
): AuditRecord["principalKind"] {
  if (value === "legacy") {
    return value;
  }
  return livePrincipalKind(value);
}

function exposureMode(value: string): "direct" | "dynamic" | "hybrid" {
  if (value === "direct" || value === "dynamic" || value === "hybrid") {
    return value;
  }
  throw new MenderClientInvariantError(`Unsupported desktop exposure mode: ${value}`);
}

function providerTransport(
  value: string,
): "http" | "stdio" | "unconfigured" {
  if (value === "http" || value === "stdio" || value === "unconfigured") {
    return value;
  }
  throw new MenderClientInvariantError(`Unsupported desktop provider transport: ${value}`);
}

function invocationKind(
  value: string,
): "complete" | "input_required" | "task" | "rejected" | "failed" {
  if (
    value === "complete" ||
    value === "input_required" ||
    value === "task" ||
    value === "rejected" ||
    value === "failed"
  ) {
    return value;
  }
  throw new MenderClientInvariantError(`Unsupported desktop invocation result kind: ${value}`);
}

function invocationFailureSource(
  value: string,
): "tool" | "provider" | "runtime" | "result_guard" {
  if (
    value === "tool" ||
    value === "provider" ||
    value === "runtime" ||
    value === "result_guard"
  ) {
    return value;
  }
  throw new MenderClientInvariantError(`Unsupported desktop invocation failure source: ${value}`);
}
