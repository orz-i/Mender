import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MCPServerInstance, MenderClient } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ServersPage } from "./servers-page";

const fixture = {
  serverId: "srv_fixture_01",
  profile: "default",
  transport: "streamable-http",
  address: "127.0.0.1:8765",
  endpoint: "http://127.0.0.1:8765/mcp",
  status: "running",
  startedAt: "2026-10-12T05:00:00Z",
  tunnelId: "tun_00000000000000000000000000000001",
  tunnelDisplayName: "OpenAI Test Tunnel",
  tunnelProvider: "openai",
  tunnelExternalId: "tunnel_test",
  tunnelStatus: "starting",
} as const satisfies MCPServerInstance;

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); vi.restoreAllMocks(); resetLocaleForTesting(); });

function captureServerStatusPolling() {
  let tickCallback: (() => void) | undefined;
  const original = window.setInterval.bind(window);
  const setIntervalSpy = vi.spyOn(window, "setInterval").mockImplementation((callback, delay) => {
    if (delay === 2000 && typeof callback === "function") {
      tickCallback = callback as () => void;
      return -877;
    }
    return original(callback, delay);
  });
  const clearSpy = vi.spyOn(window, "clearInterval");
  return {
    async tick() {
      if (!tickCallback) throw new Error("MCP server status polling was not installed");
      await act(async () => { tickCallback?.(); await Promise.resolve(); });
    },
    installed() { return tickCallback !== undefined; },
    cleared() { return clearSpy.mock.calls.some(([value]) => value === -877); },
    restore() { setIntervalSpy.mockRestore(); clearSpy.mockRestore(); },
  };
}
function showServer(
  listMCPServers: ReturnType<typeof vi.fn>,
  overrides: Partial<MenderClient> = {},
) {
  const listProfiles = vi.fn().mockResolvedValue({
    profiles: [{ profileId: "prof_00000000000000000000000000000001", slug: "default", displayName: "Default", mode: "hybrid", tools: [] }],
  });
  const client = { listProfiles, listMCPServers, ...overrides } as unknown as MenderClient;
  const view = render(
    <TooltipProvider><ServersPage client={client} /></TooltipProvider>,
  );
  return { listProfiles, ...view };
}

it("reconciles Tunnel starting -> ready -> degraded from Mender without manual Refresh", async () => {
  const polling = captureServerStatusPolling();
  try {
    const listMCPServers = vi.fn()
      .mockResolvedValueOnce({ servers: [fixture] })
      .mockResolvedValueOnce({ servers: [{ ...fixture, tunnelStatus: "ready" }] })
      .mockResolvedValueOnce({ servers: [{ ...fixture, tunnelStatus: "degraded" }] });
    const { listProfiles, unmount } = showServer(listMCPServers);
    expect(await screen.findByText("Starting")).toBeTruthy();
    await waitFor(() => expect(polling.installed()).toBe(true));
    expect(screen.getByText("Starting").className).not.toContain("text-destructive");

    await polling.tick();
    expect(await screen.findByText("Ready")).toBeTruthy();
    expect(screen.getByText("Ready").className).not.toContain("text-destructive");
    await polling.tick();
    expect(await screen.findByText("Degraded")).toBeTruthy();
    expect(screen.getByText("Degraded").className).toContain("text-destructive");
    expect(listProfiles).toHaveBeenCalledTimes(1);
    expect(listMCPServers).toHaveBeenCalledTimes(3);

    unmount();
    expect(polling.cleared()).toBe(true);
  } finally { polling.restore(); }
});

it("does not resurrect a stopped Tunnel server from an in-flight stale status request", async () => {
  const polling = captureServerStatusPolling();
  try {
    let completeStale: ((value: { servers: MCPServerInstance[] }) => void) | undefined;
    const stale = new Promise<{ servers: MCPServerInstance[] }>((resolve) => { completeStale = resolve; });
    const listMCPServers = vi.fn().mockResolvedValueOnce({ servers: [fixture] }).mockReturnValueOnce(stale);
    const stopMCPServer = vi.fn().mockResolvedValue(undefined);
    showServer(listMCPServers, { stopMCPServer });
    expect(await screen.findByText("Starting")).toBeTruthy();
    await waitFor(() => expect(polling.installed()).toBe(true));

    await polling.tick();
    fireEvent.click(screen.getByRole("button", { name: "Stop" }));
    await waitFor(() => { expect(stopMCPServer).toHaveBeenCalledWith(fixture.serverId); });
    expect(await screen.findByText("No running MCP servers")).toBeTruthy();

    await act(async () => { completeStale?.({ servers: [fixture] }); await Promise.resolve(); });
    expect(screen.getByText("No running MCP servers")).toBeTruthy();
    expect(screen.queryByText("Starting")).toBeNull();
    expect(polling.cleared()).toBe(true);
  } finally { polling.restore(); }
});

it("does not poll local-only MCP servers or install background network checks", async () => {
  const polling = captureServerStatusPolling();
  try {
    const local = { ...fixture };
    const { tunnelId: _id, tunnelDisplayName: _name, tunnelExternalId: _external, tunnelProvider: _provider, tunnelStatus: _status, ...localOnly } = local;
    void _id; void _name; void _external; void _provider; void _status;
    const listMCPServers = vi.fn().mockResolvedValue({ servers: [localOnly] });
    showServer(listMCPServers);
    expect(await screen.findByText("Local only")).toBeTruthy();
    expect(polling.installed()).toBe(false);
    expect(listMCPServers).toHaveBeenCalledTimes(1);
  } finally { polling.restore(); }
});

it("retains last known status on transient polling errors without issuing overlapping reads", async () => {
  const polling = captureServerStatusPolling();
  try {
    let finishRequest: ((value: { servers: MCPServerInstance[] }) => void) | undefined;
    const pending = new Promise<{ servers: MCPServerInstance[] }>((resolve) => { finishRequest = resolve; });
    const listMCPServers = vi.fn()
      .mockResolvedValueOnce({ servers: [fixture] })
      .mockReturnValueOnce(pending)
      .mockRejectedValueOnce(new Error("transient Wails bridge unavailable"))
      .mockResolvedValueOnce({ servers: [{ ...fixture, tunnelStatus: "ready" }] });
    showServer(listMCPServers);
    expect(await screen.findByText("Starting")).toBeTruthy();
    await waitFor(() => expect(polling.installed()).toBe(true));

    await polling.tick();
    await polling.tick();
    expect(listMCPServers).toHaveBeenCalledTimes(2);
    expect(screen.getByText("Starting")).toBeTruthy();

    await act(async () => { finishRequest?.({ servers: [fixture] }); await Promise.resolve(); });
    await polling.tick();
    expect(screen.getByText("Starting")).toBeTruthy();
    expect(screen.queryByText("No running MCP servers")).toBeNull();

    await polling.tick();
    expect(await screen.findByText("Ready")).toBeTruthy();
    expect(listMCPServers).toHaveBeenCalledTimes(4);
  } finally { polling.restore(); }
});

it("renders user-facing Tunnel phases in Chinese rather than raw English enum strings", async () => {
  setLocale("zh-CN");
  const listMCPServers = vi.fn().mockResolvedValue({ servers: [{ ...fixture, tunnelStatus: "ready" }] });
  showServer(listMCPServers);
  expect(await screen.findByText("就绪")).toBeTruthy();
  expect(screen.queryByText("ready")).toBeNull();
});
