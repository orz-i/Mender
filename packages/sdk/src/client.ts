import createClient from "openapi-fetch";

import type { components, paths } from "./generated/schema";

export type Provider = components["schemas"]["Provider"];
export type SyncResult = components["schemas"]["SyncResult"];
export type ToolSummary = components["schemas"]["ToolSummary"];
export type ToolDetail = components["schemas"]["ToolDetail"];
export type InvocationResult = components["schemas"]["InvocationResult"];
export type Profile = components["schemas"]["Profile"];
export type AuditRecord = components["schemas"]["AuditRecord"];
export type SystemStatus = components["schemas"]["SystemStatus"];
export type PolicyStatus = components["schemas"]["PolicyStatus"];
export type CredentialStatus = components["schemas"]["CredentialStatus"];
export type MCPServerInstance = components["schemas"]["MCPServerInstance"];
export type StartMCPServerRequest =
  components["schemas"]["StartMCPServerRequest"];
export type ProviderMutationResponse =
  components["schemas"]["ProviderMutationResponse"];
export type CreateHTTPProviderRequest =
  components["schemas"]["CreateHTTPProviderRequest"];
export type CreateStdioProviderRequest =
  components["schemas"]["CreateStdioProviderRequest"];
export type UpdateProviderRequest =
  components["schemas"]["UpdateProviderRequest"];
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
  listProviders(): Promise<{ providers: Provider[] }>;
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
  getCredentialStatus(): Promise<CredentialStatus>;
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

    async listProviders() {
      const result = await client.GET("/providers");
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

    async getCredentialStatus() {
      const result = await client.GET("/credentials/status");
      return requireData(result);
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
