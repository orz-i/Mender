import type {
  AuditRecord,
  CredentialStatus,
  InvocationResult,
  MenderClient,
  PolicyStatus,
  Profile,
  Provider,
  ProviderMutationResponse,
  SyncResult,
  SystemStatus,
  ToolDetail,
  ToolSummary,
} from "@mender/sdk";

import {
  Service,
  type AuditRecord as BindingAuditRecord,
  type CredentialStatus as BindingCredentialStatus,
  type InvocationResult as BindingInvocationResult,
  type PolicyStatus as BindingPolicyStatus,
  type Profile as BindingProfile,
  type Provider as BindingProvider,
  type ProviderMutationResponse as BindingProviderMutationResponse,
  type SyncResult as BindingSyncResult,
  type SystemStatus as BindingSystemStatus,
  type ToolDetail as BindingToolDetail,
  type ToolSummary as BindingToolSummary,
} from "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding";

export function createDesktopMenderClient(): MenderClient {
  return {
    async getSystemStatus() {
      return systemStatus(await Service.GetSystemStatus());
    },

    async listProviders() {
      const result = await Service.ListProviders();
      return { providers: (result.providers ?? []).map(provider) };
    },

    async createHTTPProvider(body) {
      return providerMutation(
        await Service.CreateHTTPProvider(
          body.namespace,
          body.displayName,
          body.description ?? "",
          body.endpoint,
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
        ),
      );
    },

    async syncProvider(providerId) {
      return syncResult(await Service.SyncProvider(providerId));
    },

    async deleteProvider(providerId) {
      await Service.DeleteProvider(providerId);
    },

    async searchTools(input) {
      const result = await Service.SearchTools(
        input.query,
        input.limit ?? 20,
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
        await Service.AddProfileTool(slug, body.toolId, body.direct ?? false),
      );
    },

    async removeProfileTool(slug, toolId) {
      return profile(await Service.RemoveProfileTool(slug, toolId));
    },

    async listAudit(limit = 50) {
      const result = await Service.ListAudit(limit);
      return { records: (result.records ?? []).map(auditRecord) };
    },

    async getPolicyStatus() {
      return policyStatus(await Service.GetPolicyStatus());
    },

    async getCredentialStatus() {
      return credentialStatus(await Service.GetCredentialStatus());
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
  if (value.mode !== "local_allow_all") {
    throw new Error(`Unsupported desktop policy mode: ${value.mode}`);
  }
  if (value.approval !== "fail_closed_when_required") {
    throw new Error(`Unsupported desktop approval mode: ${value.approval}`);
  }
  if (value.editable) {
    throw new Error("Desktop policy unexpectedly reported editable state");
  }
  return {
    mode: value.mode,
    editable: false,
    approval: value.approval,
  };
}

function credentialStatus(value: BindingCredentialStatus): CredentialStatus {
  if (value.resolver !== "unavailable") {
    throw new Error(
      `Unsupported desktop credential resolver: ${value.resolver}`,
    );
  }
  if (value.persistentStore || value.editable) {
    throw new Error(
      "Desktop credential state unexpectedly reported persistence or editing",
    );
  }
  return {
    resolver: value.resolver,
    persistentStore: false,
    editable: false,
  };
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
    transport,
    ...(value.endpoint === undefined ? {} : { endpoint: value.endpoint }),
    ...(value.executable === undefined
      ? {}
      : { executable: value.executable }),
    ...(value.args === undefined || value.args === null
      ? {}
      : { args: value.args }),
    ...(value.directory === undefined ? {} : { directory: value.directory }),
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
    })),
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
): "complete" | "input_required" | "task" {
  if (value === "complete" || value === "input_required" || value === "task") {
    return value;
  }
  throw new Error(`Unsupported desktop invocation result kind: ${value}`);
}
