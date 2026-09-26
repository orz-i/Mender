import { describe, expect, it } from "vitest";

import { createMenderClient } from "./client";

describe("Mender SDK", () => {
  it("uses the generated provider contract", async () => {
    const fetch = (request: Request) => {
      expect(request.url).toContain("/api/v1/providers");
      return Promise.resolve(new Response(
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
              endpoint: "http://example.test/mcp",
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ));
    };

    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch,
    });
    const result = await client.listProviders();
    expect(result.providers).toHaveLength(1);
    expect(result.providers[0]?.namespace).toBe("demo");
  });

  it("updates a Provider through the generated contract", async () => {
    const fetch = async (request: Request) => {
      expect(request.method).toBe("PUT");
      expect(request.url).toContain("/api/v1/providers/prov_1");
      await expect(request.json()).resolves.toEqual({
        transport: "http",
        displayName: "Demo edited",
        endpoint: "https://example.test/mcp",
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
            endpoint: "https://example.test/mcp",
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
    });
    expect(result.provider.displayName).toBe("Demo edited");
  });

  it("maps API error envelopes", async () => {
    const client = createMenderClient({
      baseUrl: "http://localhost:8080/api/v1",
      fetch: () =>
        Promise.resolve(new Response(
          JSON.stringify({
            code: "not_found",
            message: "tool not found",
          }),
          {
            status: 404,
            headers: { "Content-Type": "application/json" },
          },
        )),
    });

    await expect(client.getTool("tool_missing")).rejects.toMatchObject({
      name: "MenderApiError",
      code: "not_found",
      status: 404,
      message: "tool not found",
    });
  });
});
