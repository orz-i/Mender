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
