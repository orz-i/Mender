import createClient from "openapi-fetch";

import type { components, paths } from "./generated/schema";

export type ProviderPlugin = components["schemas"]["ProviderPlugin"];
export type Provider = components["schemas"]["Provider"];
export type SyncResult = components["schemas"]["SyncResult"];
export type ToolSummary = components["schemas"]["ToolSummary"];
export type ToolDetail = components["schemas"]["ToolDetail"];
export type InvocationResult = components["schemas"]["InvocationResult"];
export type Profile = components["schemas"]["Profile"];
export type AuditRecord = components["schemas"]["AuditRecord"];
export type SystemStatus = components["schemas"]["SystemStatus"];
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
  listProviderPlugins(): Promise<{ plugins: ProviderPlugin[] }>;
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
  listMCPServers(): Promise<{ servers: MCPServerInstance[] }>;
  startMCPServer(body: StartMCPServerRequest): Promise<MCPServerInstance>;
  restartMCPServer(serverId: string): Promise<MCPServerInstance>;
  stopMCPServer(serverId: string): Promise<void>;
  listAudit(limit?: number): Promise<{ records: AuditRecord[] }>;
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

    async listProviderPlugins() {
      const result = await client.GET("/provider-plugins");
      return requireData(result);
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
