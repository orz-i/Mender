import type {
  AuditRecord,
  Credential,
  CredentialStatus,
  InvocationResult,
  MCPServerInstance,
  MenderClient,
  Policy,
  PolicyRule,
  PolicyRuleInput,
  PolicyStatus,
  Profile,
  Provider,
  ProviderMutationResponse,
  ProviderPlugin,
  ProviderPluginInstallation,
  SyncResult,
  SystemStatus,
  ToolDetail,
  ToolSummary,
} from "@mender/sdk";

import {
  Service,
  type AuditRecord as BindingAuditRecord,
  type Credential as BindingCredential,
  type CredentialStatus as BindingCredentialStatus,
  type InvocationResult as BindingInvocationResult,
  type MCPServerInstance as BindingMCPServerInstance,
  type Policy as BindingPolicy,
  type PolicyRule as BindingPolicyRule,
  type PolicyRuleInput as BindingPolicyRuleInput,
  type PolicyStatus as BindingPolicyStatus,
  type Profile as BindingProfile,
  type Provider as BindingProvider,
  type ProviderMutationResponse as BindingProviderMutationResponse,
  type ProviderPlugin as BindingProviderPlugin,
  type ProviderPluginInstallation as BindingProviderPluginInstallation,
  type SyncResult as BindingSyncResult,
  type SystemStatus as BindingSystemStatus,
  type ToolDetail as BindingToolDetail,
  type ToolSummary as BindingToolSummary,
} from "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding";

export async function installDesktopProviderPlugin(
  sourcePath: string,
): Promise<ProviderPluginInstallation> {
  return providerPluginInstallation(
    await Service.InstallProviderPlugin(sourcePath),
  );
}

export function createDesktopMenderClient(): MenderClient {
  return {
    async getSystemStatus() {
      return systemStatus(await Service.GetSystemStatus());
    },

    async listProviderPlugins() {
      const result = await Service.ListProviderPlugins();
      return { plugins: (result.plugins ?? []).map(providerPlugin) };
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

function policyStatus(value: BindingPolicyStatus): PolicyStatus {
  if (value.mode !== "configured") {
    throw new Error(`Unsupported desktop policy mode: ${value.mode}`);
  }
  if (value.approval !== "fail_closed_when_required") {
    throw new Error(`Unsupported desktop approval mode: ${value.approval}`);
  }
  if (!value.editable) {
    throw new Error("Desktop policy unexpectedly reported read-only state");
  }
  if (value.ruleCount < 0 || value.enabledRuleCount < 0) {
    throw new Error("Desktop policy reported invalid Rule counts");
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
      origins: (value.match.origins ?? []).map(policyOrigin),
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
      origins: value.match.origins,
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
  throw new Error("Unsupported desktop Policy decision: " + value);
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
  throw new Error("Unsupported desktop Policy origin: " + value);
}

function policyScopeKind(
  value: string,
): PolicyRule["match"]["scopeKinds"][number] {
  if (value === "catalog" || value === "profile") {
    return value;
  }
  throw new Error("Unsupported desktop Policy scope kind: " + value);
}

function policyFactKnowledge(
  value: string,
): PolicyRule["match"]["factKnowledge"] {
  if (value === "any" || value === "known" || value === "unknown") {
    return value;
  }
  throw new Error("Unsupported desktop Policy fact knowledge: " + value);
}

function policyResourceOperator(
  value: string,
): PolicyRule["match"]["resourcesAny"][number]["operator"] {
  if (value === "exact" || value === "prefix") {
    return value;
  }
  throw new Error("Unsupported desktop Policy resource operator: " + value);
}

function credentialStatus(value: BindingCredentialStatus): CredentialStatus {
  if (value.resolver !== "inventory") {
    throw new Error(
      `Unsupported desktop credential resolver: ${value.resolver}`,
    );
  }
  if (!value.persistentStore || !value.editable) {
    throw new Error(
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
    throw new Error(`Unsupported desktop Credential source: ${value.source}`);
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

function providerPluginSourceKind(
  value: string,
): ProviderPlugin["sourceKind"] {
  if (value === "builtin" || value === "external") {
    return value;
  }
  throw new Error("Unsupported Provider plugin source kind: " + value);
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
  throw new Error("Unsupported Provider plugin config schema");
}

function providerPluginUISchema(
  value: unknown,
): NonNullable<ProviderPlugin["uiSchema"]> {
  if (typeof value === "object" && value !== null && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  throw new Error("Unsupported Provider plugin UI schema");
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
  };
}

function providerPluginInstallationStatus(
  value: string,
): ProviderPluginInstallation["status"] {
  if (value === "enabled" || value === "disabled" || value === "invalid") {
    return value;
  }
  throw new Error("Unsupported Provider plugin installation status: " + value);
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
  throw new Error("Unsupported desktop Profile policy constraint: " + value);
}

function profileConcurrencyConstraint(
  value: string,
): "inherit" | "serial" {
  if (value === "inherit" || value === "serial") {
    return value;
  }
  throw new Error("Unsupported desktop Profile concurrency constraint: " + value);
}

function mcpServerInstance(
  value: BindingMCPServerInstance,
): MCPServerInstance {
  if (value.transport !== "streamable-http") {
    throw new Error(
      `Unsupported managed MCP server transport: ${value.transport}`,
    );
  }
  if (value.status !== "running") {
    throw new Error(`Unsupported managed MCP server status: ${value.status}`);
  }
  return {
    serverId: value.serverId,
    profile: value.profile,
    transport: "streamable-http",
    address: value.address,
    endpoint: value.endpoint,
    status: "running",
    startedAt: value.startedAt,
  };
}

function auditRecord(value: BindingAuditRecord): AuditRecord {
  return {
    invocationId: value.invocationId,
    toolId: value.toolId,
    ...(value.providerId === undefined
      ? {}
      : { providerId: value.providerId }),
    caller: value.caller,
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

function exposureMode(value: string): "direct" | "dynamic" | "hybrid" {
  if (value === "direct" || value === "dynamic" || value === "hybrid") {
    return value;
  }
  throw new Error(`Unsupported desktop exposure mode: ${value}`);
}

function providerTransport(
  value: string,
): "http" | "stdio" | "unconfigured" {
  if (value === "http" || value === "stdio" || value === "unconfigured") {
    return value;
  }
  throw new Error(`Unsupported desktop provider transport: ${value}`);
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
  throw new Error(`Unsupported desktop invocation result kind: ${value}`);
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
  throw new Error(`Unsupported desktop invocation failure source: ${value}`);
}
