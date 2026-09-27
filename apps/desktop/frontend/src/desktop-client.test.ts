import type { PolicyRuleInput } from "@mender/sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock(
  "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding",
  () => ({
    Service: {
      BrowseTools: vi.fn(),
      CallTool: vi.fn(),
      CreateEnvironmentCredential: vi.fn(),
      CreateNativeCredential: vi.fn(),
      CreateProvider: vi.fn(),
      CreatePolicyRule: vi.fn(),
      DeleteCredential: vi.fn(),
      DeletePolicyRule: vi.fn(),
      GetCredential: vi.fn(),
      GetPolicy: vi.fn(),
      GetPolicyStatus: vi.fn(),
      GetSystemStatus: vi.fn(),
      ListAudit: vi.fn(),
      ListCredentials: vi.fn(),
      ListMCPServers: vi.fn(),
      ListProviderPlugins: vi.fn(),
      ListProviders: vi.fn(),
      ReplaceProfileTools: vi.fn(),
      RestartMCPServer: vi.fn(),
      RotateNativeCredential: vi.fn(),
      SearchTools: vi.fn(),
      SetPolicyDefault: vi.fn(),
      SetPolicyRuleEnabled: vi.fn(),
      StartMCPServer: vi.fn(),
      StopMCPServer: vi.fn(),
      UpdateConfiguredProvider: vi.fn(),
      UpdatePolicyRule: vi.fn(),
      UpdateProvider: vi.fn(),
    },
  }),
);

import { Service } from "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding";

import { createDesktopMenderClient } from "./desktop-client";

describe("desktop Mender client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("normalizes Wails status models into the shared client contract", async () => {
    vi.mocked(Service.GetSystemStatus).mockResolvedValue({
      ready: true,
      policy: {
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "allow",
        ruleCount: 0,
        enabledRuleCount: 0,
      },
      credentials: {
        resolver: "inventory",
        persistentStore: true,
        editable: true,
      },
      mcp: {
        stdio: true,
        streamableHttp: true,
        exposureModes: ["direct", "dynamic", "hybrid"],
      },
    });

    const client = createDesktopMenderClient();

    await expect(client.getSystemStatus()).resolves.toEqual({
      ready: true,
      policy: {
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "allow",
        ruleCount: 0,
        enabledRuleCount: 0,
      },
      credentials: {
        resolver: "inventory",
        persistentStore: true,
        editable: true,
      },
      mcp: {
        stdio: true,
        streamableHttp: true,
        exposureModes: ["direct", "dynamic", "hybrid"],
      },
    });
  });

  it("adapts generic Provider plugin discovery and configuration to Wails", async () => {
    vi.mocked(Service.ListProviderPlugins).mockResolvedValue({
      plugins: [
        {
          pluginId: "io.mender.provider.mcp",
          version: "1.0.0",
          displayName: "MCP",
          description: "Model Context Protocol provider",
          configVersion: 1,
        },
      ],
    });
    vi.mocked(Service.CreateProvider).mockResolvedValue({
      provider: {
        providerId: "prov_00000000000000000000000000000001",
        namespace: "upstream",
        displayName: "Upstream",
        description: "",
        createdAt: "2026-09-27T00:00:00Z",
        updatedAt: "2026-09-27T00:00:00Z",
        pluginId: "io.mender.provider.mcp",
        pluginVersion: "1.0.0",
        pluginDisplayName: "MCP",
        configVersion: 1,
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://example.test/mcp",
          },
        },
        transport: "http",
        toolCount: 1,
        endpoint: "https://example.test/mcp",
      },
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    const client = createDesktopMenderClient();
    await expect(client.listProviderPlugins()).resolves.toEqual({
      plugins: [
        {
          pluginId: "io.mender.provider.mcp",
          version: "1.0.0",
          displayName: "MCP",
          description: "Model Context Protocol provider",
          configVersion: 1,
        },
      ],
    });

    const config = {
      transport: {
        kind: "streamable-http",
        endpoint: "https://example.test/mcp",
      },
    };
    await expect(
      client.createProvider({
        namespace: "upstream",
        displayName: "Upstream",
        pluginId: "io.mender.provider.mcp",
        config,
      }),
    ).resolves.toMatchObject({
      provider: {
        pluginId: "io.mender.provider.mcp",
        config,
        transport: "http",
      },
    });
    expect(Service.CreateProvider).toHaveBeenCalledWith(
      "upstream",
      "Upstream",
      "",
      "io.mender.provider.mcp",
      config,
    );
  });

  it("normalizes nullable binding arrays without leaking Wails models", async () => {
    vi.mocked(Service.ListProviders).mockResolvedValue({
      providers: [
        {
          providerId: "prov_00000000000000000000000000000001",
          namespace: "upstream",
          displayName: "Upstream",
          description: "",
          createdAt: "2026-09-26T00:00:00Z",
          updatedAt: "2026-09-26T00:00:00Z",
          transport: "stdio",
          toolCount: 1,
          executable: "example",
          args: null,
        },
      ],
    });

    const client = createDesktopMenderClient();

    await expect(client.listProviders()).resolves.toEqual({
      providers: [
        {
          providerId: "prov_00000000000000000000000000000001",
          namespace: "upstream",
          displayName: "Upstream",
          description: "",
          createdAt: "2026-09-26T00:00:00Z",
          updatedAt: "2026-09-26T00:00:00Z",
          transport: "stdio",
          toolCount: 1,
          executable: "example",
        },
      ],
    });
  });

  it("adapts Provider updates and header templates to the Wails binding", async () => {
    vi.mocked(Service.UpdateProvider).mockResolvedValue({
      provider: {
        providerId: "prov_00000000000000000000000000000001",
        namespace: "upstream",
        displayName: "Upstream edited",
        description: "Edited",
        createdAt: "2026-09-26T00:00:00Z",
        updatedAt: "2026-09-26T01:00:00Z",
        transport: "http",
        toolCount: 1,
        endpoint: "https://example.test/mcp",
        headers: {
          "X-Legacy": "${EXAMPLE_METADATA}",
        },
        credentialHeaders: [
          {
            name: "Authorization",
            scheme: "Bearer",
            credentialRef: "credential://github/work",
          },
        ],
      },
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });

    const client = createDesktopMenderClient();
    const result = await client.updateProvider(
      "prov_00000000000000000000000000000001",
      {
        transport: "http",
        displayName: "Upstream edited",
        description: "Edited",
        endpoint: "https://example.test/mcp",
        headers: {
          "X-Legacy": "${EXAMPLE_METADATA}",
        },
        credentialHeaders: [
          {
            name: "Authorization",
            scheme: "Bearer",
            credentialRef: "credential://github/work",
          },
        ],
      },
    );

    expect(Service.UpdateProvider).toHaveBeenCalledWith(
      "prov_00000000000000000000000000000001",
      "Upstream edited",
      "Edited",
      "http",
      "https://example.test/mcp",
      {
        "X-Legacy": "${EXAMPLE_METADATA}",
      },
      [
        {
          name: "Authorization",
          scheme: "Bearer",
          credentialRef: "credential://github/work",
        },
      ],
      "",
      [],
      "",
      [],
    );
    expect(result.provider.displayName).toBe("Upstream edited");
    expect(result.provider.toolCount).toBe(1);
    expect(result.provider.headers).toEqual({
      "X-Legacy": "${EXAMPLE_METADATA}",
    });
    expect(result.provider.credentialHeaders).toEqual([
      {
        name: "Authorization",
        scheme: "Bearer",
        credentialRef: "credential://github/work",
      },
    ]);
  });

  it("browses compact synchronized Tools through Wails", async () => {
    vi.mocked(Service.BrowseTools).mockResolvedValue({
      tools: [
        {
          rank: 1,
          toolId: "tool_1",
          providerId: "prov_1",
          name: "search",
          title: "",
          description: "Search",
        },
      ],
    });

    const client = createDesktopMenderClient();

    await expect(client.browseTools()).resolves.toEqual({
      tools: [
        {
          rank: 1,
          toolId: "tool_1",
          providerId: "prov_1",
          name: "search",
          title: "",
          description: "Search",
        },
      ],
    });
    expect(Service.BrowseTools).toHaveBeenCalledWith(100, "");
  });

  it("forwards Provider-scoped Tool search through Wails", async () => {
    vi.mocked(Service.SearchTools).mockResolvedValue({
      tools: [
        {
          rank: 1,
          toolId: "tool_1",
          providerId: "prov_1",
          name: "search",
          title: "",
          description: "Search",
        },
      ],
    });

    const client = createDesktopMenderClient();
    await expect(
      client.searchTools({
        query: "search",
        limit: 30,
        providerId: "prov_1",
      }),
    ).resolves.toMatchObject({
      tools: [{ toolId: "tool_1", providerId: "prov_1" }],
    });

    expect(Service.SearchTools).toHaveBeenCalledWith(
      "search",
      30,
      "prov_1",
      "",
    );
  });

  it("maps Credential inventory without exposing secret values", async () => {
    vi.mocked(Service.ListCredentials).mockResolvedValue({
      credentials: [
        {
          reference: "credential://demo/work",
          namespace: "demo",
          name: "work",
          displayName: "Demo work",
          source: "environment",
          environmentVariable: "DEMO_WORK_TOKEN",
          createdAt: "2026-09-27T00:00:00Z",
          updatedAt: "2026-09-27T00:00:00Z",
        },
      ],
    });
    vi.mocked(Service.CreateEnvironmentCredential).mockResolvedValue({
      reference: "credential://demo/work",
      namespace: "demo",
      name: "work",
      displayName: "Demo work",
      source: "environment",
      environmentVariable: "DEMO_WORK_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    });

    const client = createDesktopMenderClient();

    await expect(client.listCredentials()).resolves.toEqual({
      credentials: [
        {
          reference: "credential://demo/work",
          namespace: "demo",
          name: "work",
          displayName: "Demo work",
          source: "environment",
          environmentVariable: "DEMO_WORK_TOKEN",
          createdAt: "2026-09-27T00:00:00Z",
          updatedAt: "2026-09-27T00:00:00Z",
        },
      ],
    });
    await expect(
      client.createEnvironmentCredential({
        namespace: "demo",
        name: "work",
        displayName: "Demo work",
        environmentVariable: "DEMO_WORK_TOKEN",
      }),
    ).resolves.toMatchObject({
      reference: "credential://demo/work",
      source: "environment",
    });
    await client.deleteCredential("demo", "work");

    expect(Service.CreateEnvironmentCredential).toHaveBeenCalledWith(
      "demo",
      "work",
      "Demo work",
      "DEMO_WORK_TOKEN",
    );
    expect(Service.DeleteCredential).toHaveBeenCalledWith("demo", "work");
  });

  it("maps persistent Policy CRUD through Wails", async () => {
    const rule = {
      ruleId: "rule_00000000000000000000000000000001",
      displayName: "Guard MCP",
      enabled: true,
      decision: "deny",
      match: {
        origins: ["mcp"],
        scopeKinds: ["profile"],
        profileRefs: ["prof_00000000000000000000000000000001"],
        providerIds: ["prov_00000000000000000000000000000001"],
        toolIds: ["tool_00000000000000000000000000000001"],
        factKnowledge: "known",
        effectsAny: ["filesystem.write"],
        resourcesAny: [
          { kind: "path", operator: "prefix", value: "/workspace/release" },
        ],
      },
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    vi.mocked(Service.GetPolicy).mockResolvedValue({
      defaultDecision: "allow",
      rules: [rule],
    });
    vi.mocked(Service.SetPolicyDefault).mockResolvedValue({
      defaultDecision: "deny",
      rules: [rule],
    });
    vi.mocked(Service.CreatePolicyRule).mockResolvedValue(rule);
    vi.mocked(Service.UpdatePolicyRule).mockResolvedValue({
      ...rule,
      displayName: "Updated guard",
    });
    vi.mocked(Service.SetPolicyRuleEnabled).mockResolvedValue({
      ...rule,
      enabled: false,
    });

    const client = createDesktopMenderClient();
    const input: PolicyRuleInput = {
      displayName: "Guard MCP",
      enabled: true,
      decision: "deny" as const,
      match: {
        origins: ["mcp"],
        scopeKinds: ["profile"],
        profileRefs: ["prof_00000000000000000000000000000001"],
        providerIds: ["prov_00000000000000000000000000000001"],
        toolIds: ["tool_00000000000000000000000000000001"],
        factKnowledge: "known",
        effectsAny: ["filesystem.write"],
        resourcesAny: [
          {
            kind: "path",
            operator: "prefix",
            value: "/workspace/release",
          },
        ],
      },
    };

    await expect(client.getPolicy()).resolves.toMatchObject({
      defaultDecision: "allow",
      rules: [{ decision: "deny", match: { origins: ["mcp"] } }],
    });
    await expect(
      client.setPolicyDefault({ decision: "deny" }),
    ).resolves.toMatchObject({ defaultDecision: "deny" });
    await expect(client.createPolicyRule(input)).resolves.toMatchObject({
      ruleId: rule.ruleId,
      decision: "deny",
    });
    await expect(
      client.updatePolicyRule(rule.ruleId, {
        ...input,
        displayName: "Updated guard",
      }),
    ).resolves.toMatchObject({ displayName: "Updated guard" });
    await expect(
      client.setPolicyRuleEnabled(rule.ruleId, false),
    ).resolves.toMatchObject({ enabled: false });
    await client.deletePolicyRule(rule.ruleId);

    expect(Service.SetPolicyDefault).toHaveBeenCalledWith("deny");
    expect(Service.CreatePolicyRule).toHaveBeenCalledWith(
      expect.objectContaining({
        displayName: "Guard MCP",
        decision: "deny",
      }),
    );
    expect(Service.UpdatePolicyRule).toHaveBeenCalledWith(
      rule.ruleId,
      expect.objectContaining({ displayName: "Updated guard" }),
    );
    expect(Service.SetPolicyRuleEnabled).toHaveBeenCalledWith(
      rule.ruleId,
      false,
    );
    expect(Service.DeletePolicyRule).toHaveBeenCalledWith(rule.ruleId);
  });

  it("replaces Profile Tool membership through Wails", async () => {
    vi.mocked(Service.ReplaceProfileTools).mockResolvedValue({
      profileId: "prof_1",
      slug: "default",
      displayName: "Default",
      mode: "hybrid",
      tools: [
        {
          toolId: "tool_1",
          direct: true,
          alias: "profile_tool",
          titleOverride: "Profile Tool",
          descriptionOverride: "Profile-specific presentation",
          policyConstraint: "deny",
          concurrencyConstraint: "serial",
          credentialRefs: ["credential://github/work"],
        },
        {
          toolId: "tool_2",
          direct: false,
          alias: "",
          titleOverride: "",
          descriptionOverride: "",
          policyConstraint: "inherit",
          concurrencyConstraint: "inherit",
          credentialRefs: [],
        },
      ],
    });

    const client = createDesktopMenderClient();
    const result = await client.replaceProfileTools("default", {
      tools: [
        {
          toolId: "tool_1",
          direct: true,
          alias: "profile_tool",
          titleOverride: "Profile Tool",
          descriptionOverride: "Profile-specific presentation",
          policyConstraint: "deny",
          concurrencyConstraint: "serial",
          credentialRefs: ["credential://github/work"],
        },
        { toolId: "tool_2", direct: false },
      ],
    });

    expect(Service.ReplaceProfileTools).toHaveBeenCalledWith("default", [
      {
        toolId: "tool_1",
        direct: true,
        alias: "profile_tool",
        titleOverride: "Profile Tool",
        descriptionOverride: "Profile-specific presentation",
        policyConstraint: "deny",
        concurrencyConstraint: "serial",
        credentialRefs: ["credential://github/work"],
      },
      {
        toolId: "tool_2",
        direct: false,
        alias: "",
        titleOverride: "",
        descriptionOverride: "",
        policyConstraint: "inherit",
        concurrencyConstraint: "inherit",
        credentialRefs: [],
      },
    ]);
    expect(result.tools).toHaveLength(2);
    expect(result.tools[0]).toMatchObject({
      alias: "profile_tool",
      policyConstraint: "deny",
      concurrencyConstraint: "serial",
    });
  });

  it("adapts managed MCP server lifecycle to Wails", async () => {
    const instance = {
      serverId: "srv_1",
      profile: "default",
      transport: "streamable-http",
      address: "127.0.0.1:8765",
      endpoint: "http://127.0.0.1:8765/mcp",
      status: "running",
      startedAt: "2026-09-26T09:00:00Z",
    };
    vi.mocked(Service.ListMCPServers).mockResolvedValue({
      servers: [instance],
    });
    vi.mocked(Service.StartMCPServer).mockResolvedValue(instance);
    vi.mocked(Service.RestartMCPServer).mockResolvedValue({
      ...instance,
      serverId: "srv_2",
    });
    vi.mocked(Service.StopMCPServer).mockResolvedValue(undefined);

    const client = createDesktopMenderClient();

    await expect(client.listMCPServers()).resolves.toEqual({
      servers: [instance],
    });
    await expect(
      client.startMCPServer({
        profile: "default",
        address: "127.0.0.1:8765",
      }),
    ).resolves.toEqual(instance);
    expect(Service.StartMCPServer).toHaveBeenCalledWith(
      "default",
      "127.0.0.1:8765",
    );

    await expect(client.restartMCPServer("srv_1")).resolves.toMatchObject({
      serverId: "srv_2",
    });
    await client.stopMCPServer("srv_2");
    expect(Service.StopMCPServer).toHaveBeenCalledWith("srv_2");
  });

  it("maps governed invocation outcomes and Audit metadata through Wails", async () => {
    vi.mocked(Service.CallTool).mockResolvedValue({
      invocationId: "inv_1",
      kind: "failed",
      message: "tool result does not match the declared output schema",
      source: "result_guard",
      code: "output_schema_mismatch",
    });
    vi.mocked(Service.ListAudit).mockResolvedValue({
      records: [
        {
          invocationId: "inv_1",
          toolId: "tool_1",
          providerId: "prov_1",
          caller: "desktop",
          startedAt: "2026-09-26T00:00:00Z",
          finishedAt: "2026-09-26T00:00:01Z",
          policyKind: "allow",
          policyReason: "local.default",
          approval: "not_required",
          status: "failed",
          resultKind: "",
          errorClass: "result_guard",
          errorSummary: "",
          origin: "desktop",
          scopeKind: "catalog",
          scopeReference: "",
          outcomeKind: "failed",
          rejectionCode: "",
          failureSource: "result_guard",
          failureCode: "output_schema_mismatch",
        },
      ],
    });

    const client = createDesktopMenderClient();

    await expect(client.callTool("tool_1", {})).resolves.toEqual({
      invocationId: "inv_1",
      kind: "failed",
      message: "tool result does not match the declared output schema",
      source: "result_guard",
      code: "output_schema_mismatch",
    });
    await expect(client.listAudit(10)).resolves.toMatchObject({
      records: [
        {
          origin: "desktop",
          scopeKind: "catalog",
          outcomeKind: "failed",
          failureSource: "result_guard",
          failureCode: "output_schema_mismatch",
        },
      ],
    });
  });

  it("rejects unexpected binding enum values at the host boundary", async () => {
    vi.mocked(Service.ListProviders).mockResolvedValue({
      providers: [
        {
          providerId: "prov_00000000000000000000000000000001",
          namespace: "upstream",
          displayName: "Upstream",
          description: "",
          createdAt: "2026-09-26T00:00:00Z",
          updatedAt: "2026-09-26T00:00:00Z",
          transport: "unknown",
          toolCount: 0,
        },
      ],
    });

    const client = createDesktopMenderClient();

    await expect(client.listProviders()).rejects.toThrow(
      "Unsupported desktop provider transport",
    );
  });
});
