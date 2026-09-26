import { describe, expect, it } from "vitest";

import { createMenderClient } from "./client";

describe("Mender SDK", () => {
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
        transport: "http",
        displayName: "Demo edited",
        endpoint: "https://example.test/mcp",
        headers: {
          Authorization: "Bearer ${EXAMPLE_API_KEY}",
        },
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
      transport: "http",
      displayName: "Demo edited",
      endpoint: "https://example.test/mcp",
      headers: {
        Authorization: "Bearer ${EXAMPLE_API_KEY}",
      },
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

  it("replaces Profile Tool membership through the generated contract", async () => {
    const fetch = async (request: Request) => {
      expect(request.method).toBe("PUT");
      expect(request.url).toContain("/api/v1/profiles/default/tools");
      await expect(request.json()).resolves.toEqual({
        tools: [
          { toolId: "tool_1", direct: true },
          { toolId: "tool_2", direct: false },
        ],
      });
      return new Response(
        JSON.stringify({
          profileId: "prof_1",
          slug: "default",
          displayName: "Default",
          mode: "hybrid",
          tools: [
            { toolId: "tool_1", direct: true },
            { toolId: "tool_2", direct: false },
          ],
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
      client.replaceProfileTools("default", {
        tools: [
          { toolId: "tool_1", direct: true },
          { toolId: "tool_2", direct: false },
        ],
      }),
    ).resolves.toMatchObject({
      slug: "default",
      tools: [
        { toolId: "tool_1", direct: true },
        { toolId: "tool_2", direct: false },
      ],
    });
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
});
