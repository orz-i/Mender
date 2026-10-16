import type { Provider, UpdateProviderRequest } from "@mender/sdk";
import { t } from "../../i18n/i18n";

export const MCP_PLUGIN_ID = "io.mender.provider.mcp";
export const MODEL_TOOLS_PLUGIN_ID = "io.mender.provider.model-tools";
export const OPENAPI_PLUGIN_ID = "io.mender.provider.openapi";
export const WORKSPACE_PLUGIN_ID = "io.mender.provider.workspace";
export const AGENT_DELEGATION_PLUGIN_ID = "io.mender.provider.agent-delegation";
export const SANDBOX_JOBS_PLUGIN_ID = "io.mender.provider.sandbox-jobs";
export function isInternalProvider(pluginId:string|undefined){
  return pluginId===AGENT_DELEGATION_PLUGIN_ID || pluginId===WORKSPACE_PLUGIN_ID || pluginId===SANDBOX_JOBS_PLUGIN_ID;
}

type ProviderTransport = "http" | "stdio";
type ModelToolsEngine = "openai" | "anthropic" | "gemini";
type OpenAPISourceKind = "url" | "file";
type CredentialHeaderBinding = NonNullable<Provider["credentialHeaders"]>[number];
type CredentialEnvironmentBinding = NonNullable<
  Provider["credentialEnvironment"]
>[number];

interface OpenAPICredentialBinding {
  schemeName: string;
  credentialRef: string;
}

export interface ProviderFormState {
  pluginId: string;
  installationId: string;
  genericConfig: Record<string, unknown>;
  transport: ProviderTransport;
  namespace: string;
  displayName: string;
  description: string;
  endpoint: string;
  headers: string;
  credentialHeaders: CredentialHeaderBinding[];
  importJson: string;
  executable: string;
  args: string;
  directory: string;
  credentialEnvironment: CredentialEnvironmentBinding[];
  openapiSourceKind: OpenAPISourceKind;
  openapiSpecUrl: string;
  openapiSpecFile: string;
  openapiServerUrl: string;
  openapiInclude: string;
  openapiExclude: string;
  openapiCredentials: OpenAPICredentialBinding[];
  modelToolsEngine: ModelToolsEngine;
  modelToolsModel: string;
  modelToolsCredentialRef: string;
}

type ProviderRenderer = "mcp" | "model-tools" | "openapi" | "generic";
type FirstPartyProviderRenderer = Exclude<ProviderRenderer, "generic">;

const FIRST_PARTY_PROVIDER_RENDERERS: Partial<
  Record<string, FirstPartyProviderRenderer>
> = {
  [MCP_PLUGIN_ID]: "mcp",
  [MODEL_TOOLS_PLUGIN_ID]: "model-tools",
  [OPENAPI_PLUGIN_ID]: "openapi",
};

export function providerRenderer(form: ProviderFormState): ProviderRenderer {
  if (form.installationId) {
    return "generic";
  }
  return FIRST_PARTY_PROVIDER_RENDERERS[form.pluginId] ?? "generic";
}

export const emptyProviderForm: ProviderFormState = {
  pluginId: MCP_PLUGIN_ID,
  installationId: "",
  genericConfig: {},
  transport: "http",
  namespace: "",
  displayName: "",
  description: "",
  endpoint: "",
  headers: "",
  credentialHeaders: [],
  importJson: "",
  executable: "",
  args: "",
  directory: "",
  credentialEnvironment: [],
  openapiSourceKind: "url",
  openapiSpecUrl: "",
  openapiSpecFile: "",
  openapiServerUrl: "",
  openapiInclude: "",
  openapiExclude: "",
  openapiCredentials: [],
  modelToolsEngine: "openai",
  modelToolsModel: "",
  modelToolsCredentialRef: "",
};

export function formFromProvider(provider: Provider): ProviderFormState {
  const config = asRecord(provider.config);
  const source = asRecord(config?.source);
  const operations = asRecord(config?.operations);
  const credentialConfig = asRecord(config?.credentials);
  const sourceKind: OpenAPISourceKind =
    source?.kind === "file" ? "file" : "url";

  const openapiCredentials: OpenAPICredentialBinding[] = credentialConfig
    ? Object.entries(credentialConfig)
        .filter((entry): entry is [string, string] => typeof entry[1] === "string")
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([schemeName, credentialRef]) => ({ schemeName, credentialRef }))
    : [];

  return {
    pluginId: provider.pluginId ?? MCP_PLUGIN_ID,
    installationId: provider.installationId ?? "",
    genericConfig: config ?? {},
    transport: provider.transport === "stdio" ? "stdio" : "http",
    namespace: provider.namespace,
    displayName: provider.displayName,
    description: provider.description,
    endpoint: provider.endpoint ?? "",
    headers: formatHeaders(provider.headers ?? {}),
    credentialHeaders: [...(provider.credentialHeaders ?? [])],
    importJson: "",
    executable: provider.executable ?? "",
    args: provider.args?.join("\n") ?? "",
    directory: provider.directory ?? "",
    credentialEnvironment: [...(provider.credentialEnvironment ?? [])],
    openapiSourceKind: sourceKind,
    openapiSpecUrl: typeof source?.url === "string" ? source.url : "",
    openapiSpecFile: typeof source?.path === "string" ? source.path : "",
    openapiServerUrl:
      typeof config?.serverUrl === "string" ? config.serverUrl : "",
    openapiInclude: stringListFromUnknown(operations?.include).join("\n"),
    openapiExclude: stringListFromUnknown(operations?.exclude).join("\n"),
    openapiCredentials,
    modelToolsEngine: modelToolsEngineFromUnknown(config?.engine),
    modelToolsModel: typeof config?.model === "string" ? config.model : "",
    modelToolsCredentialRef:
      typeof config?.credentialRef === "string" ? config.credentialRef : "",
  };
}

export function updateRequestFromForm(form: ProviderFormState): UpdateProviderRequest {
  return {
    displayName: form.displayName,
    ...(form.description ? { description: form.description } : {}),
    config: configFromForm(form),
  };
}

export function configFromForm(form: ProviderFormState): Record<string, unknown> {
  const renderer = providerRenderer(form);
  if (renderer === "generic") {
    return JSON.parse(JSON.stringify(form.genericConfig)) as Record<string, unknown>;
  }

  if (renderer === "model-tools") {
    if (!form.modelToolsCredentialRef) {
      throw new Error(t("Choose a Model API Credential."));
    }
    return {
      engine: form.modelToolsEngine,
      model: form.modelToolsModel.trim(),
      credentialRef: form.modelToolsCredentialRef,
      capabilities: ["web_search"],
    };
  }

  if (renderer === "openapi") {
    const include = parseLines(form.openapiInclude);
    const exclude = parseLines(form.openapiExclude);
    const credentials: Record<string, string> = {};
    for (const binding of form.openapiCredentials) {
      const schemeName = binding.schemeName.trim();
      if (!schemeName) {
        throw new Error(t("OpenAPI Security Scheme name is required."));
      }
      if (credentials[schemeName] !== undefined) {
        throw new Error(t('OpenAPI Security Scheme "{{name}}" is configured more than once.', { name: schemeName }));
      }
      if (!binding.credentialRef) {
        throw new Error(t('Choose a Credential for OpenAPI Security Scheme "{{name}}".', { name: schemeName }));
      }
      credentials[schemeName] = binding.credentialRef;
    }

    return {
      source:
        form.openapiSourceKind === "url"
          ? { kind: "url", url: form.openapiSpecUrl.trim() }
          : { kind: "file", path: form.openapiSpecFile.trim() },
      ...(form.openapiServerUrl.trim()
        ? { serverUrl: form.openapiServerUrl.trim() }
        : {}),
      ...(include.length || exclude.length
        ? {
            operations: {
              ...(include.length ? { include } : {}),
              ...(exclude.length ? { exclude } : {}),
            },
          }
        : {}),
      ...(Object.keys(credentials).length ? { credentials } : {}),
    };
  }

  if (renderer !== "mcp") {
    throw new Error(t("Unsupported Provider renderer."));
  }
  if (form.transport === "http") {
    const headers = parseHeaders(form.headers);
    return {
      transport: {
        kind: "streamable-http",
        endpoint: form.endpoint,
        ...(Object.keys(headers).length ? { headers } : {}),
        ...(form.credentialHeaders.length
          ? { credentialHeaders: form.credentialHeaders }
          : {}),
      },
    };
  }

  const args = parseArgs(form.args);
  return {
    transport: {
      kind: "stdio",
      executable: form.executable,
      ...(form.directory ? { directory: form.directory } : {}),
      ...(args.length ? { args } : {}),
      ...(form.credentialEnvironment.length
        ? { credentialEnvironment: form.credentialEnvironment }
        : {}),
    },
  };
}

function parseHeaders(value: string): Record<string, string> {
  const result: Record<string, string> = {};
  const lines = value.split("\n");

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]?.trim() ?? "";
    if (!line) {
      continue;
    }
    const separator = line.indexOf(":");
    if (separator <= 0) {
      throw new Error(
        t('Header line {{line}} must use "Name: value" format.', { line: index + 1 }),
      );
    }
    const name = line.slice(0, separator).trim();
    const headerValue = line.slice(separator + 1).trim();
    if (!name) {
      throw new Error(t("Header line {{line}} has an empty name.", { line: index + 1 }));
    }
    result[name] = headerValue;
  }

  return result;
}

export function formatHeaders(headers: Record<string, string>): string {
  return Object.entries(headers)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, value]) => `${name}: ${value}`)
    .join("\n");
}

export function parseMCPJSON(value: string): {
  namespace: string;
  endpoint: string;
  headers: Record<string, string>;
} {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    throw new Error(t("MCP JSON must be valid JSON."));
  }

  const root = asRecord(parsed);
  if (root === null) {
    throw new Error(t("MCP JSON must contain an object."));
  }

  let entries: [string, unknown][];
  const mcpServers = asRecord(root.mcpServers);
  const mcp = asRecord(root.mcp);
  if (mcpServers !== null) {
    entries = Object.entries(mcpServers);
  } else if (mcp !== null) {
    entries = Object.entries(mcp);
  } else if (
    typeof root.url === "string" ||
    typeof root.endpoint === "string"
  ) {
    entries = [["provider", root]];
  } else {
    throw new Error(t('Expected an "mcpServers" or "mcp" object.'));
  }

  if (entries.length !== 1) {
    throw new Error(t("Paste a configuration containing exactly one MCP server."));
  }

  const [namespace, rawConfig] = entries[0]!;
  const config = asRecord(rawConfig);
  if (config === null) {
    throw new Error(t("MCP server configuration must be an object."));
  }

  if (typeof config.type === "string") {
    const type = config.type.toLowerCase();
    if (!["remote", "streamable-http", "http"].includes(type)) {
      throw new Error(
        t('Unsupported remote MCP type "{{type}}". Expected remote or streamable-http.', { type: config.type }),
      );
    }
  }

  const endpoint =
    typeof config.url === "string"
      ? config.url
      : typeof config.endpoint === "string"
        ? config.endpoint
        : "";
  if (!endpoint) {
    throw new Error(t('Remote MCP configuration requires "url".'));
  }

  const headers: Record<string, string> = {};
  if (config.headers !== undefined) {
    const rawHeaders = asRecord(config.headers);
    if (rawHeaders === null) {
      throw new Error(t('"headers" must be an object of string values.'));
    }
    for (const [name, headerValue] of Object.entries(rawHeaders)) {
      if (typeof headerValue !== "string") {
        throw new Error(t('Header "{{name}}" must have a string value.', { name }));
      }
      headers[name] = headerValue;
    }
  }

  return { namespace, endpoint, headers };
}

export function asRecord(value: unknown): Record<string, unknown> | null {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function parseArgs(value: string) {
  return parseLines(value);
}

function parseLines(value: string) {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function stringListFromUnknown(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
}

function modelToolsEngineFromUnknown(value: unknown): ModelToolsEngine {
  return value === "anthropic" || value === "gemini" ? value : "openai";
}

export function modelToolsEngineLabel(engine: ModelToolsEngine) {
  switch (engine) {
    case "openai":
      return "OpenAI";
    case "anthropic":
      return "Anthropic";
    case "gemini":
      return "Gemini";
  }
}

export function modelToolsModelPlaceholder(engine: ModelToolsEngine) {
  switch (engine) {
    case "openai":
      return "gpt-5.5";
    case "anthropic":
      return "claude-opus-5-5";
    case "gemini":
      return "gemini-3.8-flash";
  }
}

export function modelToolsEngineSummary(provider: Provider) {
  const config = asRecord(provider.config);
  return modelToolsEngineLabel(modelToolsEngineFromUnknown(config?.engine));
}

export function modelToolsModelSummary(provider: Provider) {
  const config = asRecord(provider.config);
  return typeof config?.model === "string" && config.model
    ? config.model
    : "—";
}

export function openAPISourceSummary(provider: Provider) {
  const config = asRecord(provider.config);
  const source = asRecord(config?.source);
  if (source?.kind === "file" && typeof source.path === "string") {
    return t("File · {{path}}", { path: source.path });
  }
  if (typeof source?.url === "string") {
    return t("URL · {{url}}", { url: source.url });
  }
  return "—";
}

export function openAPIServerSummary(provider: Provider) {
  const config = asRecord(provider.config);
  return typeof config?.serverUrl === "string" && config.serverUrl
    ? config.serverUrl
    : t("Document server");
}

export function openAPIFilterSummary(provider: Provider) {
  const config = asRecord(provider.config);
  const operations = asRecord(config?.operations);
  const include = stringListFromUnknown(operations?.include);
  const exclude = stringListFromUnknown(operations?.exclude);
  if (!include.length && !exclude.length) {
    return t("All supported operations");
  }
  const lines = [
    ...(include.length ? [t("Include: {{value}}", { value: include.join(", ") })] : []),
    ...(exclude.length ? [t("Exclude: {{value}}", { value: exclude.join(", ") })] : []),
  ];
  return lines.join("\n");
}

export function openAPISecuritySummary(provider: Provider) {
  const config = asRecord(provider.config);
  const credentials = asRecord(config?.credentials);
  if (credentials === null) {
    return t("None configured");
  }
  const names = Object.keys(credentials).sort((left, right) =>
    left.localeCompare(right),
  );
  return names.length ? names.join("\n") : t("None configured");
}

export function connectionSummary(provider: Provider) {
  if (provider.connectionSummary?.trim()) {
    return provider.connectionSummary;
  }
  if (provider.installationId) {
    return t("External plugin · {{installation}}", { installation: provider.installationId });
  }
  if (provider.pluginId === MODEL_TOOLS_PLUGIN_ID) {
    return (
      modelToolsEngineSummary(provider) +
      " · " +
      modelToolsModelSummary(provider)
    );
  }
  if (provider.pluginId === OPENAPI_PLUGIN_ID) {
    const config = asRecord(provider.config);
    const source = asRecord(config?.source);
    if (source?.kind === "file" && typeof source.path === "string") {
      return source.path;
    }
    if (typeof source?.url === "string") {
      return source.url;
    }
    return t("OpenAPI source not configured");
  }
  if (provider.transport === "http") {
    return provider.endpoint ?? t("not configured");
  }
  if (provider.transport === "stdio") {
    return provider.executable ?? t("not configured");
  }
  return t("not configured");
}

