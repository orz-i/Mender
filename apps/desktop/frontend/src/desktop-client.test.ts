import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock(
  "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding",
  () => ({
    Service: {
      BrowseTools: vi.fn(),
      GetSystemStatus: vi.fn(),
      ListMCPServers: vi.fn(),
      ListProviders: vi.fn(),
      RestartMCPServer: vi.fn(),
      StartMCPServer: vi.fn(),
      StopMCPServer: vi.fn(),
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
        mode: "local_allow_all",
        editable: false,
        approval: "fail_closed_when_required",
      },
      credentials: {
        resolver: "unavailable",
        persistentStore: false,
        editable: false,
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
        mode: "local_allow_all",
        editable: false,
        approval: "fail_closed_when_required",
      },
      credentials: {
        resolver: "unavailable",
        persistentStore: false,
        editable: false,
      },
      mcp: {
        stdio: true,
        streamableHttp: true,
        exposureModes: ["direct", "dynamic", "hybrid"],
      },
    });
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
          Authorization: "Bearer ${EXAMPLE_API_KEY}",
        },
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
          Authorization: "Bearer ${EXAMPLE_API_KEY}",
        },
      },
    );

    expect(Service.UpdateProvider).toHaveBeenCalledWith(
      "prov_00000000000000000000000000000001",
      "Upstream edited",
      "Edited",
      "http",
      "https://example.test/mcp",
      {
        Authorization: "Bearer ${EXAMPLE_API_KEY}",
      },
      "",
      [],
      "",
    );
    expect(result.provider.displayName).toBe("Upstream edited");
    expect(result.provider.toolCount).toBe(1);
    expect(result.provider.headers).toEqual({
      Authorization: "Bearer ${EXAMPLE_API_KEY}",
    });
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
