import type { Provider } from "@mender/sdk";
import { describe, expect, it } from "vitest";

import {
  MCP_PLUGIN_ID,
  MODEL_TOOLS_PLUGIN_ID,
  OPENAPI_PLUGIN_ID,
  configFromForm,
  connectionSummary,
  emptyProviderForm,
  formFromProvider,
  parseMCPJSON,
  providerRenderer,
  updateRequestFromForm,
} from "./provider-form-model";

describe("pure Provider form adapters", () => {
  it("normalizes built-in MCP remote and local transport without changing credential refs", () => {
    const http = {
      ...emptyProviderForm,
      endpoint: "https://mcp.example.test/mcp",
      headers: "X-Client: mender\nX-Mode: test",
      credentialHeaders: [{ name: "Authorization", credentialRef: "credential://demo/token", scheme: "bearer" }],
    };
    expect(providerRenderer(http)).toBe("mcp");
    expect(configFromForm(http)).toEqual({
      transport: {
        kind: "streamable-http",
        endpoint: "https://mcp.example.test/mcp",
        headers: { "X-Client": "mender", "X-Mode": "test" },
        credentialHeaders: http.credentialHeaders,
      },
    });
    const stdio = {
      ...emptyProviderForm,
      transport: "stdio" as const,
      executable: "/usr/bin/mcp",
      directory: "/tmp/work",
      args: "--mode=read\n\n--max=10",
      credentialEnvironment: [{ name: "TOKEN", credentialRef: "credential://demo/token" }],
    };
    expect(configFromForm(stdio)).toEqual({
      transport: {
        kind: "stdio", executable: "/usr/bin/mcp", directory: "/tmp/work",
        args: ["--mode=read", "--max=10"],
        credentialEnvironment: stdio.credentialEnvironment,
      },
    });
    expect(parseMCPJSON(JSON.stringify({
      mcpServers: { docs: { type: "streamable-http", url: http.endpoint, headers: { "X-Mode": "test" } } },
    }))).toEqual({ namespace: "docs", endpoint: http.endpoint, headers: { "X-Mode": "test" } });
    expect(() => parseMCPJSON('{"mcpServers":{"a":{"url":"https://x"},"b":{"url":"https://y"}}}'))
      .toThrow("Paste a configuration containing exactly one MCP server.");
  });

  it("maps OpenAPI operations and credentials explicitly, refusing duplicate Security Schemes", () => {
    const form = {
      ...emptyProviderForm, pluginId: OPENAPI_PLUGIN_ID,
      openapiSourceKind: "url" as const,
      openapiSpecUrl: " https://example.test/openapi.yaml ",
      openapiServerUrl: " https://example.test/api ",
      openapiInclude: "listPets\ngetPet",
      openapiExclude: "deletePet",
      openapiCredentials: [{ schemeName: "bearerAuth", credentialRef: "credential://demo/api" }],
    };
    expect(configFromForm(form)).toEqual({
      source: { kind: "url", url: "https://example.test/openapi.yaml" },
      serverUrl: "https://example.test/api",
      operations: { include: ["listPets", "getPet"], exclude: ["deletePet"] },
      credentials: { bearerAuth: "credential://demo/api" },
    });
    expect(() => configFromForm({ ...form, openapiCredentials: [form.openapiCredentials[0]!, form.openapiCredentials[0]!] }))
      .toThrow('OpenAPI Security Scheme "bearerAuth" is configured more than once.');
  });

  it("preserves model-tools and generic exact resource bindings", () => {
    expect(() => configFromForm({ ...emptyProviderForm, pluginId: MODEL_TOOLS_PLUGIN_ID }))
      .toThrow("Choose a Model API Credential.");
    expect(configFromForm({
      ...emptyProviderForm, pluginId: MODEL_TOOLS_PLUGIN_ID,
      modelToolsEngine: "anthropic", modelToolsModel: " test-model ",
      modelToolsCredentialRef: "credential://demo/anthropic",
    })).toEqual({
      engine: "anthropic", model: "test-model",
      credentialRef: "credential://demo/anthropic",
      capabilities: ["web_search"],
    });
    const custom = {
      ...emptyProviderForm, pluginId: "dev.example.provider", installationId: "pinst_fixture",
      genericConfig: { nested: { retries: 2 } },
    };
    const serialized = configFromForm(custom);
    expect(providerRenderer(custom)).toBe("generic");
    expect(serialized).toEqual(custom.genericConfig);
    expect(serialized).not.toBe(custom.genericConfig);
  });

  it("restores an existing Provider edit model with stable identity and presentation fields", () => {
    const provider = {
      providerId: "prov_fixture",
      namespace: "internal",
      displayName: "Docs MCP",
      description: "Existing",
      pluginId: MCP_PLUGIN_ID,
      transport: "http",
      endpoint: "https://mcp.example.test/mcp",
      headers: { "X-Mode": "read" },
      credentialHeaders: [],
    } as unknown as Provider;
    const form = formFromProvider(provider);
    expect(form.namespace).toBe("internal");
    expect(form.pluginId).toBe(MCP_PLUGIN_ID);
    expect(form.headers).toBe("X-Mode: read");
    expect(updateRequestFromForm(form)).toMatchObject({
      displayName: "Docs MCP", description: "Existing",
      config: { transport: { kind: "streamable-http", endpoint: "https://mcp.example.test/mcp", headers: { "X-Mode": "read" } } },
    });
    expect(connectionSummary(provider)).toBe("https://mcp.example.test/mcp");
  });
});
