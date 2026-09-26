import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock(
  "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding",
  () => ({
    Service: {
      GetSystemStatus: vi.fn(),
      ListProviders: vi.fn(),
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
          executable: "example",
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
        },
      ],
    });

    const client = createDesktopMenderClient();

    await expect(client.listProviders()).rejects.toThrow(
      "Unsupported desktop provider transport",
    );
  });
});
