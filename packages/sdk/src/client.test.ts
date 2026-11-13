import { describe, expect, it } from "vitest";

import { createMenderClient } from "./client";
import type { Profile, ProfileMCPDelivery, ReplaceProfileToolsRequest } from "./contracts/management-client";

describe("Mender SDK", () => {
  it("exposes only common management operations over HTTP", () => {
    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1" });

    for (const desktopOperation of [
      "listAgentApprovals", "decideAgentApproval", "listPendingAgentApprovals",
      "getAgentApprovalPolicy", "listAgentApprovalPolicies", "setAgentApprovalPolicy",
      "releasePendingAgentApprovals", "listAgentToolPlacements", "configureAgentToolPlacement",
      "disableAgentToolPlacement", "getLocalProfileCLI", "openExternalURL",
    ]) {
      expect(client).not.toHaveProperty(desktopOperation);
    }
  });

  it("discovers Provider plugins and creates a generic Provider", async () => {
    let call = 0;
    const fetch = async (request: Request) => {
      call += 1;
      if (call == 1) {
        expect(request.method).toBe("GET");
        expect(request.url).toContain("/api/v1/provider-plugins");
        return new Response(
          JSON.stringify({
            plugins: [
              {
                pluginId: "io.mender.provider.mcp",
                version: "1.0.0",
                displayName: "MCP",
                description: "Model Context Protocol provider",
                configVersion: 1,
              },
            ],
            issues: [],
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }

      expect(request.method).toBe("POST");
      expect(request.url).toContain("/api/v1/providers");
      await expect(request.json()).resolves.toEqual({
        namespace: "demo",
        displayName: "Demo",
        pluginId: "io.mender.provider.mcp",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://example.test/mcp",
          },
        },
      });
      return new Response(
        JSON.stringify({
          provider: {
            providerId: "prov_1",
            namespace: "demo",
            displayName: "Demo",
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
            endpoint: "https://example.test/mcp",
            toolCount: 1,
          },
          sync: { created: 1, updated: 0, removed: 0, total: 1 },
        }),
        {
          status: 201,
          headers: { "Content-Type": "application/json" },
        },
      );
    };

    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch,
    });

    await expect(client.listProviderPlugins()).resolves.toMatchObject({
      plugins: [{ pluginId: "io.mender.provider.mcp" }],
      issues: [],
    });
    await expect(
      client.createProvider({
        namespace: "demo",
        displayName: "Demo",
        pluginId: "io.mender.provider.mcp",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://example.test/mcp",
          },
        },
      }),
    ).resolves.toMatchObject({
      provider: {
        pluginId: "io.mender.provider.mcp",
        toolCount: 1,
      },
    });
  });

  it("rebinds a Provider to an exact external plugin installation", async () => {
    const fetch = async (request: Request) => {
      expect(request.method).toBe("POST");
      expect(request.url).toContain("/api/v1/providers/prov_external/rebind");
      await expect(request.json()).resolves.toEqual({
        installationId: "pinst_22222222222222222222222222222222",
        config: { prefix: "next: " },
      });
      return new Response(
        JSON.stringify({
          provider: {
            providerId: "prov_external",
            namespace: "external",
            displayName: "External",
            description: "",
            createdAt: "2026-09-28T00:00:00Z",
            updatedAt: "2026-09-28T00:00:00Z",
            pluginId: "dev.example.echo",
            installationId: "pinst_22222222222222222222222222222222",
            configVersion: 1,
            connectionSummary: "Example Echo",
            config: { prefix: "next: " },
            transport: "unconfigured",
            toolCount: 1,
          },
          sync: { created: 0, updated: 1, removed: 0, total: 1 },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );
    };
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch,
    });

    await expect(
      client.rebindProvider("prov_external", {
        installationId: "pinst_22222222222222222222222222222222",
        config: { prefix: "next: " },
      }),
    ).resolves.toMatchObject({
      provider: {
        providerId: "prov_external",
        installationId: "pinst_22222222222222222222222222222222",
      },
      sync: { updated: 1 },
    });
  });

  it("uses the generated provider contract", async () => {
    const fetch = (request: Request) => {
      expect(request.url).toContain("/api/v1/providers");
      return Promise.resolve(
        new Response(
          JSON.stringify({
            providers: [
              {
                providerId: "prov_1",
                namespace: "demo",
                displayName: "Demo",
                description: "",
                createdAt: "2026-09-26T00:00:00Z",
                updatedAt: "2026-09-26T00:00:00Z",
                transport: "http",
                toolCount: 4,
                endpoint: "https://example.test/mcp",
                headers: {
                  Authorization: "Bearer ${EXAMPLE_API_KEY}",
                },
              },
            ],
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      );
    };

    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch,
    });
    const result = await client.listProviders();
    expect(result.providers).toHaveLength(1);
    expect(result.providers[0]?.namespace).toBe("demo");
    expect(result.providers[0]?.toolCount).toBe(4);
  });

  it("updates a Provider through the generated header-template contract", async () => {
    const fetch = async (request: Request) => {
      expect(request.method).toBe("PUT");
      expect(request.url).toContain("/api/v1/providers/prov_1");
      await expect(request.json()).resolves.toEqual({
        displayName: "Demo edited",
        config: {transport: {kind: "streamable-http", endpoint: "https://example.test/mcp",
          headers: {Authorization: "Bearer TEST_TOKEN"},
        }},
      });
      return new Response(
        JSON.stringify({
          provider: {
            providerId: "prov_1",
            namespace: "demo",
            displayName: "Demo edited",
            description: "",
            createdAt: "2026-09-26T00:00:00Z",
            updatedAt: "2026-09-26T01:00:00Z",
            transport: "http",
            toolCount: 1,
            endpoint: "https://example.test/mcp",
            headers: {
              Authorization: "Bearer ${EXAMPLE_API_KEY}",
            },
          },
          sync: { created: 0, updated: 1, removed: 0, total: 1 },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );
    };

    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch,
    });

    const result = await client.updateProvider("prov_1", {
      displayName: "Demo edited",
      config: {transport: {kind: "streamable-http", endpoint: "https://example.test/mcp",
        headers: {Authorization: "Bearer TEST_TOKEN"},
      }},
    });
    expect(result.provider.displayName).toBe("Demo edited");
  });

  it("browses compact synchronized Tools without a search query", async () => {
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: (request) => {
        expect(request.url).toContain("/api/v1/tools/browse");
        expect(request.url).toContain("limit=100");
        return Promise.resolve(
          new Response(
            JSON.stringify({
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
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      },
    });

    const result = await client.browseTools({ limit: 100 });
    expect(result.tools).toHaveLength(1);
    expect(result.tools[0]?.name).toBe("search");
  });

  it("searches Tools within a Provider scope", async () => {
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: (request) => {
        expect(request.url).toContain("/api/v1/tools");
        expect(request.url).toContain("query=search");
        expect(request.url).toContain("sourceId=provider%3Aprov_1");
        expect(request.url).not.toContain("profile=");
        return Promise.resolve(
          new Response(
            JSON.stringify({
              tools: [
                {
                  rank: 1,
                  toolId: "tool_1",
                  sourceId: "provider:prov_1",
                  sourceKind: "provider",
                  providerId: "prov_1",
                  name: "search",
                  title: "",
                  description: "Search",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      },
    });

    const result = await client.searchTools({
      query: "search",
      limit: 30,
      sourceId: "provider:prov_1",
    });
    expect(result.tools).toHaveLength(1);
  });

  it("replaces Profile Tool membership through the generated contract", async () => {
    const input: ReplaceProfileToolsRequest = { tools: [
      { toolId: "tool_1", alias: "chosen_alias", policyConstraint: "deny", credentialRefs: ["credential://github/work"] },
      { toolId: "tool_2" },
    ] };
    const profile: Profile = {
      profileId: "prof_1", slug: "default", displayName: "Default", skills: [], revision: 0,
      tools: [
        { toolId: "tool_1", alias: "chosen_alias", titleOverride: "", descriptionOverride: "", policyConstraint: "deny", concurrencyConstraint: "inherit", credentialRefs: ["credential://github/work"] },
        { toolId: "tool_2", alias: "", titleOverride: "", descriptionOverride: "", policyConstraint: "inherit", concurrencyConstraint: "inherit", credentialRefs: [] },
      ],
    };
    const fetch = async (request: Request) => {
      expect(request.method).toBe("PUT");
      expect(request.url).toContain("/api/v1/profiles/default/tools");
      await expect(request.json()).resolves.toEqual(input);
      return new Response(
        JSON.stringify(profile),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );
    };

    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch,
    });

    const updated = await client.replaceProfileTools("default", input);
    expect(updated).toEqual(profile);
    expect(updated).not.toHaveProperty("mode");
    expect(updated).not.toHaveProperty("tunnelId");
    expect(updated.tools.every((tool) => !("direct" in tool))).toBe(true);
  });

  it("returns null only for an explicitly unconfigured MCP delivery", async () => {
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: async (request) => {
        expect(request.method).toBe("GET");
        expect(new URL(request.url).pathname).toBe("/api/v1/profiles/selected/mcp-delivery");
        expect(await request.text()).toBe("");
        return new Response(JSON.stringify({ code: "mcp_delivery_not_configured", message: "MCP delivery: not configured" }), {
          status: 404, headers: { "Content-Type": "application/json" },
        });
      },
    });
    await expect(client.getProfileMCPDelivery("selected")).resolves.toBeNull();
  });

  it.each([
    { status: 404, code: "not_found", message: "Profile not found" },
    { status: 409, code: "revision_conflict", message: "Delivery revision changed" },
    { status: 500, code: "mcp_delivery_not_configured", message: "Storage unavailable" },
    { status: 403, code: "forbidden", message: "Owner required" },
  ])("preserves MCP delivery HTTP errors ($status/$code)", async ({ status, code, message }) => {
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: () => Promise.resolve(new Response(JSON.stringify({ code, message }), {
        status, headers: { "Content-Type": "application/json" },
      })),
    });
    await expect(client.getProfileMCPDelivery("selected")).rejects.toMatchObject({
      name: "MenderApiError", status, code, message,
    });
  });

  it("does not treat an unknown 404 envelope or transport failure as unconfigured", async () => {
    const unknown = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: () => Promise.resolve(new Response(JSON.stringify({ error: { code: "mcp_delivery_not_configured" } }), {
        status: 404, headers: { "Content-Type": "application/json" },
      })),
    });
    await expect(unknown.getProfileMCPDelivery("selected")).rejects.toMatchObject({
      name: "MenderApiError", code: "http_error", status: 404,
    });
    const transportError = new Error("connection lost");
    const unavailable = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1", fetch: () => Promise.reject(transportError),
    });
    await expect(unavailable.getProfileMCPDelivery("selected")).rejects.toBe(transportError);
  });

  it("preserves exact MCP delivery identity and both zero revision fences", async () => {
    const tunnelId = "tun_0123456789abcdef0123456789abcdef";
    const response: ProfileMCPDelivery = {
      profileId: "prof_0123456789abcdef0123456789abcdef", profileRevision: 0, revision: 0,
      mode: "hybrid", tunnelId,
      directToolIds: ["tool_0123456789abcdef0123456789abcdef"],
    };
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: async (request) => {
        expect(request.method).toBe("PUT");
        expect(new URL(request.url).pathname).toBe("/api/v1/profiles/selected/mcp-delivery");
        await expect(request.json()).resolves.toEqual({
          mode: response.mode, tunnelId: response.tunnelId, directToolIds: response.directToolIds,
          expectedProfileRevision: 0, expectedRevision: 0,
        });
        return new Response(JSON.stringify(response), { status: 200, headers: { "Content-Type": "application/json" } });
      },
    });
    await expect(client.configureProfileMCPDelivery("selected", {
      mode: response.mode, tunnelId, directToolIds: response.directToolIds,
      expectedProfileRevision: 0, expectedRevision: 0,
    })).resolves.toEqual(response);
  });

  it("omits only the delivery revision fence for an explicit create", async () => {
    const response: ProfileMCPDelivery = { profileId: "prof_1", profileRevision: 0, revision: 0, mode: "dynamic", directToolIds: [] };
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: async (request) => {
        const input: unknown = await request.json();
        expect(input).toEqual({ mode: "dynamic", directToolIds: [], expectedProfileRevision: 0 });
        expect(input).not.toHaveProperty("expectedRevision");
        return new Response(JSON.stringify(response), { status: 200, headers: { "Content-Type": "application/json" } });
      },
    });
    await expect(client.configureProfileMCPDelivery("selected", {
      mode: "dynamic", directToolIds: [], expectedProfileRevision: 0,
    })).resolves.toEqual(response);
  });

  it("replaces Profile Skill membership through the typed generated contract", async () => {
    const id = "sinst_00000000000000000000000000000001";
    const fetch = async (request: Request) => {
      expect(request.method).toBe("PUT");
      expect(request.url).toContain("/api/v1/profiles/default/skills");
      await expect(request.json()).resolves.toEqual({ installationIds: [id] });
      return new Response(JSON.stringify({
        profileId: "prof_1", slug: "default", displayName: "Default",
        revision: 0, tools: [], skills: [id],
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    };
    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    const updated = await client.replaceProfileSkills("default", { installationIds: [id] });
    expect(updated.skills).toEqual([id]);
  });

  it("queries only Profile-owned Skill deployments through the generated contract", async () => {
    const fetch = async (request: Request) => {
      expect(request.method).toBe("GET");
      expect(request.url).toContain("/api/v1/profiles/delivery/skill-deployments");
      expect(await request.text()).toBe("");
      return new Response(JSON.stringify({ deployments: [{
        deploymentId: "sdep_1", installationId: "sinst_1", profileId: "prof_1",
        profileRevision: 3, target: "agents-project", root: "/tmp",
        destination: "/tmp/.agents/skills/example", digest: "sha256:" + "a".repeat(64),
        deployedAt: "2026-10-23T00:00:00Z", drift: "current",
      }] }), { status: 200, headers: { "Content-Type": "application/json" } });
    };
    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    await expect(client.listProfileSkillDeployments("delivery")).resolves.toMatchObject({
      deployments: [{ profileRevision: 3, profileId: "prof_1" }],
    });
  });

  it("undeploys a Skill through its exact Profile path", async () => {
    const fetch = async (request: Request) => {
      expect(request.method).toBe("DELETE");
      expect(request.url).toContain("/api/v1/profiles/delivery/skill-deployments/sdep_123");
      expect(await request.text()).toBe("");
      return new Response(null, { status: 204 });
    };
    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    await expect(client.undeployProfileSkill("delivery", "sdep_123")).resolves.toBeUndefined();
  });

  it("starts a managed MCP server through the generated contract", async () => {
    const fetch = async (request: Request) => {
      expect(request.method).toBe("POST");
      expect(request.url).toContain("/api/v1/mcp/servers");
      await expect(request.json()).resolves.toEqual({
        profile: "default",
        address: "127.0.0.1:8765",
      });
      return new Response(
        JSON.stringify({
          serverId: "srv_1",
          profile: "default",
          transport: "streamable-http",
          address: "127.0.0.1:8765",
          endpoint: "http://127.0.0.1:8765/mcp",
          status: "running",
          startedAt: "2026-09-26T09:00:00Z",
        }),
        {
          status: 201,
          headers: { "Content-Type": "application/json" },
        },
      );
    };

    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch,
    });

    await expect(
      client.startMCPServer({
        profile: "default",
        address: "127.0.0.1:8765",
      }),
    ).resolves.toMatchObject({
      serverId: "srv_1",
      endpoint: "http://127.0.0.1:8765/mcp",
    });
  });

  it("manages Skill inventory and deployments through the generated contract", async () => {
    const installationId = "sinst_0123456789abcdef0123456789abcdef";
    const deploymentId = "sdep_0123456789abcdef0123456789abcdef";
    let call = 0;
    const fetch = (request: Request) => {
      call += 1;
      if (call === 1) {
        expect(request.method).toBe("GET");
        expect(request.url).toContain("/api/v1/skills");
        return Promise.resolve(
          new Response(
            JSON.stringify({
              installations: [
                {
                  skillId: "skill_0123456789abcdef0123456789abcdef",
                  installationId,
                  name: "mender-tools",
                  description: "Mender tools",
                  sourceKind: "builtin",
                  sourceReference: "skills/mender-tools",
                  digest: "sha256:" + "a".repeat(64),
                  versionHint: "",
                  trusted: true,
                  enabled: true,
                  builtIn: true,
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      if (call === 2) {
        expect(request.method).toBe("POST");
        expect(request.url).toContain(
          "/api/v1/skills/" + installationId + "/trust",
        );
        return Promise.resolve(
          new Response(
            JSON.stringify({
              installation: {
                skillId: "skill_0123456789abcdef0123456789abcdef",
                installationId,
                name: "example-skill",
                description: "Example",
                sourceKind: "local-directory",
                sourceReference: "/tmp/example-skill",
                digest: "sha256:" + "b".repeat(64),
                versionHint: "1.0.0",
                trusted: true,
                enabled: false,
                builtIn: false,
                installedAt: "2026-09-28T00:00:00Z",
              },
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      if (call === 3) {
        expect(request.method).toBe("GET");
        expect(request.url).toContain("/api/v1/skill-deployments");
        return Promise.resolve(
          new Response(
            JSON.stringify({
              deployments: [
                {
                  deploymentId,
                  installationId,
                  target: "agents-project",
                  root: "/workspace",
                  destination: "/workspace/.agents/skills/example-skill",
                  digest: "sha256:" + "b".repeat(64),
                  deployedAt: "2026-09-28T00:00:00Z",
                  drift: "current",
                  currentDigest: "sha256:" + "b".repeat(64),
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }

      expect(request.method).toBe("DELETE");
      expect(request.url).toContain(
        "/api/v1/skill-deployments/" + deploymentId,
      );
      return Promise.resolve(new Response(null, { status: 204 }));
    };

    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch,
    });

    await expect(client.listSkills()).resolves.toMatchObject({
      installations: [{ name: "mender-tools", builtIn: true }],
    });
    await expect(
      client.trustSkillInstallation(installationId),
    ).resolves.toMatchObject({
      installation: { trusted: true, enabled: false },
    });
    await expect(client.listSkillDeployments()).resolves.toMatchObject({
      deployments: [{ deploymentId, drift: "current" }],
    });
    await expect(client.undeploySkill(deploymentId)).resolves.toBeUndefined();
    expect(call).toBe(4);
  });

  it("maps API error envelopes", async () => {
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: () =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              code: "not_found",
              message: "tool not found",
            }),
            {
              status: 404,
              headers: { "Content-Type": "application/json" },
            },
          ),
        ),
    });

    await expect(client.getTool("tool_missing")).rejects.toMatchObject({
      name: "MenderApiError",
      code: "not_found",
      status: 404,
      message: "tool not found",
    });
  });

  it("retrieves transcript events for an Agent task", async () => {
    let call = 0;
    const fetch = async (request: Request) => {
      call += 1;
      expect(request.method).toBe("GET");
      expect(request.url).toContain("/api/v1/agent-tasks/atask_001/transcript");
      return new Response(
        JSON.stringify({
          events: [
            {
              eventId: "evt_1",
              taskId: "atask_001",
              seq: 1,
              type: "user_prompt",
              payload: { prompt: "Analyze project health" },
              createdAt: "2026-11-12T10:00:00Z",
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );
    };

    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    const { events } = await client.getAgentTaskTranscript("atask_001");
    expect(call).toBe(1);
    expect(events).toHaveLength(1);
    expect(events[0]?.type).toBe("user_prompt");
    expect(events[0]?.payload.prompt).toBe("Analyze project health");
  });

  it("subscribes to Agent task events via EventSource", () => {
    const originalEventSource = globalThis.EventSource;
    let urlCaptured = "";
    let closed = false;
    let mockInstance: any = null;

    class MockEventSource {
      url: string;
      onmessage: ((ev: any) => void) | null = null;
      onerror: ((ev: any) => void) | null = null;
      constructor(url: string) {
        this.url = url;
        urlCaptured = url;
        mockInstance = this;
      }
      addEventListener(type: string, listener: any) {}
      close() {
        closed = true;
      }
    }
    // @ts-expect-error mock
    globalThis.EventSource = MockEventSource;

    try {
      const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1" });
      const received: any[] = [];
      const unsub = client.subscribeAgentTaskEvents("atask_001", (ev) => {
        received.push(ev);
      });

      expect(urlCaptured).toBe("http://localhost:8080/api/v1/agent-tasks/atask_001/events");
      expect(mockInstance).not.toBeNull();

      // Simulate incoming event
      mockInstance.onmessage({
        data: JSON.stringify({
          eventId: "evt_100",
          taskId: "atask_001",
          seq: 1,
          type: "message_chunk",
          payload: { text: "Streaming chunk" },
          createdAt: "2026-11-12T10:00:00Z",
        }),
      });

      expect(received).toHaveLength(1);
      expect(received[0].payload.text).toBe("Streaming chunk");

      unsub();
      expect(closed).toBe(true);
    } finally {
      globalThis.EventSource = originalEventSource;
    }
  });

  it("submits a new Agent task with options via HTTP client", async () => {
    let call = 0;
    const fetch: (req: Request) => Promise<Response> = async (req) => {
      call++;
      expect(req.url).toBe("http://localhost:8080/api/v1/agents/agent_001/tasks");
      expect(req.method).toBe("POST");
      const body = await req.json();
      expect(body).toEqual({
        prompt: "Build feature X",
        title: "Feature X",
        model: "test-model",
        reasoningEffort: "high",
        workspace: "default",
        timeoutMinutes: 20,
      });
      return new Response(
        JSON.stringify({
          taskId: "atask_002",
          agentId: "agent_001",
          status: "queued",
          title: "Feature X",
          model: "test-model",
          reasoningEffort: "high",
          timeoutSeconds: 1200,
          workspace: "default",
          createdAt: "2026-11-12T10:00:00Z",
          updatedAt: "2026-11-12T10:00:00Z",
          cancelRequested: false,
        }),
        {
          status: 201,
          headers: { "Content-Type": "application/json" },
        },
      );
    };

    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    const task = await client.submitAgentTask("agent_001", "Build feature X", {
      title: "Feature X",
      model: "test-model",
      reasoningEffort: "high",
      workspace: "default",
      timeoutMinutes: 20,
    });
    expect(call).toBe(1);
    expect(task.taskId).toBe("atask_002");
    expect(task.status).toBe("queued");
    expect(task.timeoutSeconds).toBe(1200);
  });

  it("resumes an Agent task session with follow-up prompt via HTTP client", async () => {
    let call = 0;
    const fetch: (req: Request) => Promise<Response> = async (req) => {
      call++;
      expect(req.url).toBe("http://localhost:8080/api/v1/agent-tasks/atask_002/resume");
      expect(req.method).toBe("POST");
      const body = await req.json();
      expect(body).toEqual({
        prompt: "Now write tests for it",
      });
      return new Response(
        JSON.stringify({
          taskId: "atask_003",
          agentId: "agent_001",
          status: "queued",
          continuedFromTaskId: "atask_002",
          timeoutSeconds: 600,
          createdAt: "2026-11-12T10:05:00Z",
          updatedAt: "2026-11-12T10:05:00Z",
          cancelRequested: false,
        }),
        {
          status: 201,
          headers: { "Content-Type": "application/json" },
        },
      );
    };

    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    const task = await client.resumeAgentTask("atask_002", "Now write tests for it");
    expect(call).toBe(1);
    expect(task.taskId).toBe("atask_003");
    expect(task.continuedFromTaskId).toBe("atask_002");
    expect(task.status).toBe("queued");
  });

  it("triggers registry sync via syncRegistry", async () => {
    let call = 0;
    const fetch: (req: Request) => Promise<Response> = async (req) => {
      call++;
      expect(req.url).toBe("http://localhost:8080/api/v1/registry/sync");
      expect(req.method).toBe("POST");
      return new Response(JSON.stringify({ synced: true, count: 5 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    };
    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    const res = await client.syncRegistry();
    expect(call).toBe(1);
    expect(res).toEqual({ synced: true, count: 5 });
  });

  it("installs an online plugin via installRegistryPlugin", async () => {
    let call = 0;
    const fetch: (req: Request) => Promise<Response> = async (req) => {
      call++;
      expect(req.url).toBe("http://localhost:8080/api/v1/registry/plugins/install");
      expect(req.method).toBe("POST");
      const body = await req.json();
      expect(body).toEqual({ pluginId: "dev.example.websearch", version: "1.0.0" });
      return new Response(
        JSON.stringify({
          installation: {
            installationId: "pinst_reg_001",
            pluginId: "dev.example.websearch",
            status: "enabled",
            packageVersion: "1.0.0",
            sourceKind: "registry",
          },
        }),
        {
          status: 201,
          headers: { "Content-Type": "application/json" },
        },
      );
    };
    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    const res = await client.installRegistryPlugin({ pluginId: "dev.example.websearch", version: "1.0.0" });
    expect(call).toBe(1);
    expect(res.installation.installationId).toBe("pinst_reg_001");
  });

  it("upgrades an installed plugin via upgradeRegistryPlugin", async () => {
    let call = 0;
    const fetch: (req: Request) => Promise<Response> = async (req) => {
      call++;
      expect(req.url).toBe("http://localhost:8080/api/v1/registry/plugins/upgrade");
      expect(req.method).toBe("POST");
      const body = await req.json();
      expect(body).toEqual({ installationId: "pinst_reg_001", version: "1.1.0" });
      return new Response(
        JSON.stringify({
          installation: {
            installationId: "pinst_reg_001",
            pluginId: "dev.example.websearch",
            status: "enabled",
            packageVersion: "1.1.0",
            sourceKind: "registry",
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );
    };
    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    const res = await client.upgradeRegistryPlugin({ installationId: "pinst_reg_001", version: "1.1.0" });
    expect(call).toBe(1);
    expect(res.installation.packageVersion).toBe("1.1.0");
  });

  it("lists available updates via listRegistryUpdates", async () => {
    let call = 0;
    const fetch: (req: Request) => Promise<Response> = async (req) => {
      call++;
      expect(req.url).toBe("http://localhost:8080/api/v1/registry/updates");
      expect(req.method).toBe("GET");
      return new Response(
        JSON.stringify({
          updates: [
            {
              installationId: "pinst_reg_001",
              pluginId: "dev.example.websearch",
              currentVersion: "1.0.0",
              targetVersion: "1.1.0",
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );
    };
    const client = createMenderClient({ baseUrl: "http://localhost:8080/api/v1", fetch });
    const res = await client.listRegistryUpdates();
    expect(call).toBe(1);
    expect(res.updates).toHaveLength(1);
    expect(res.updates[0]!.targetVersion).toBe("1.1.0");
  });
});
