import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

it.each([
  { provider: "openai" as const, label: "OpenAI" },
  { provider: "external" as const, label: "External plugin" },
])("shows only the Tunnel name for a healthy $provider Tunnel", async ({ provider, label }) => {
  showServer(vi.fn().mockResolvedValue({
    servers: [{ ...fixture, tunnelDisplayName: "Mender", tunnelProvider: provider, tunnelStatus: "ready" }],
  }));

  const row = await screen.findByRole("row", { name: /default/ });
  const tunnelCell = within(row).getAllByRole("cell")[2]!;
  const tunnelName = within(tunnelCell).getByText("Mender");
  expect(tunnelCell.textContent).toBe("Mender");
  expect(tunnelName.getAttribute("title")).toBe("Mender · " + label + " · Ready");
  expect(tunnelCell.querySelector('[data-slot="badge"]')).toBeNull();
  expect(row.querySelectorAll('[data-slot="badge"]')).toHaveLength(1); // Server running only
});

it("does not assume an OpenAI Tunnel provider when provider metadata is absent", async () => {
  const { tunnelProvider: _provider, ...withoutProvider } = fixture;
  void _provider;
  showServer(vi.fn().mockResolvedValue({
    servers: [{ ...withoutProvider, tunnelDisplayName: "Mender", tunnelStatus: "ready" }],
  }));
  const row = await screen.findByRole("row", { name: /default/ });
  const tunnelCell = within(row).getAllByRole("cell")[2]!;
  expect(tunnelCell.textContent).toBe("Mender");
  expect(within(tunnelCell).getByText("Mender").getAttribute("title")).toBe("Mender · Ready");
  expect(within(row).queryByText("OpenAI")).toBeNull();
});

it.each([
  { status: "degraded" as const, variant: "bg-sunlight/20" },
  { status: "failed" as const, variant: "bg-destructive/10" },
  { status: "exited" as const, variant: "bg-sunlight/20" },
])("surfaces the $status Tunnel exception next to its name", async ({ status, variant }) => {
  showServer(vi.fn().mockResolvedValue({
    servers: [{ ...fixture, tunnelDisplayName: "Mender", tunnelStatus: status }],
  }));
  const row = await screen.findByRole("row", { name: /default/ });
  const tunnelCell = within(row).getAllByRole("cell")[2]!;
  expect(within(tunnelCell).getByText("Mender")).toBeTruthy();
  const warning = tunnelCell.querySelector('[data-slot="badge"]');
  expect(warning?.className).toContain(variant);
  expect(warning?.textContent).toBe(status === "degraded" ? "Degraded" : status === "failed" ? "Failed" : "Exited");
});

it("reconciles Tunnel starting -> ready -> degraded from Mender without manual Refresh", async () => {
  const polling = captureServerStatusPolling();
  try {
    const listMCPServers = vi.fn()
      .mockResolvedValueOnce({ servers: [fixture] })
      .mockResolvedValueOnce({ servers: [{ ...fixture, tunnelStatus: "ready" }] })
      .mockResolvedValueOnce({ servers: [{ ...fixture, tunnelStatus: "degraded" }] });
    const { listProfiles, unmount } = showServer(listMCPServers);
    const tunnel = await screen.findByText("OpenAI Test Tunnel");
    expect(tunnel.getAttribute("title")).toContain("Starting");
    expect(screen.queryByText("Starting")).toBeNull();
    await waitFor(() => expect(polling.installed()).toBe(true));

    await polling.tick();
    await waitFor(() => expect(tunnel.getAttribute("title")).toContain("Ready"));
    expect(screen.queryByText("Ready")).toBeNull();
    await polling.tick();
    expect(await screen.findByText("Degraded")).toBeTruthy();
    // Degraded remains visibly actionable without being conflated with a terminal failure.
    expect(screen.getByText("Degraded").className).toContain("bg-sunlight/20");
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
    expect(await screen.findByText("OpenAI Test Tunnel")).toBeTruthy();
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

it("requires an explicit Profile when multiple exist and preserves inputs after a start error", async () => {
  const profiles = [
    { profileId: "prof_alpha", slug: "alpha", displayName: "Alpha", mode: "hybrid", tools: [] },
    { profileId: "prof_beta", slug: "beta", displayName: "Beta", mode: "hybrid", tools: [] },
  ];
  const startMCPServer = vi.fn().mockRejectedValue(new Error("Address is already in use"));
  showServer(vi.fn().mockResolvedValue({ servers: [] }), {
    listProfiles: vi.fn().mockResolvedValue({ profiles }),
    startMCPServer,
  });
  expect(await screen.findByText("No running MCP servers")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Start server" }));
  const form = within(screen.getByRole("dialog", { name: "Start MCP server" }));
  expect(form.getByLabelText<HTMLSelectElement>("Server profile").value).toBe("");
  expect(form.getByRole<HTMLButtonElement>("button", { name: "Start server" }).disabled).toBe(true);
  expect(form.getByText("Advanced settings").closest("details")?.open).toBe(false);
  expect(form.getByText("127.0.0.1:8765")).toBeTruthy();
  fireEvent.change(form.getByLabelText("Server profile"), { target: { value: "beta" } });
  fireEvent.click(form.getByText("Advanced settings"));
  expect(form.getByLabelText<HTMLInputElement>("Listen address").value).toBe("127.0.0.1:8765");
  fireEvent.click(form.getByRole("button", { name: "Start server" }));
  expect((await form.findByRole("alert")).textContent).toContain("already in use");
  expect(startMCPServer).toHaveBeenCalledWith({ profile: "beta", address: "127.0.0.1:8765" });
  expect(form.getByLabelText<HTMLSelectElement>("Server profile").value).toBe("beta");
  expect(form.getByLabelText<HTMLInputElement>("Listen address").value).toBe("127.0.0.1:8765");
});

it("displays the real rotated endpoint without asking the user to search for a free port", async () => {
  const current = { ...fixture, profile: "alpha", tunnelId: undefined, tunnelStatus: undefined,
    tunnelDisplayName: undefined, tunnelProvider: undefined, tunnelExternalId: undefined };
  const rotated = { ...current, serverId: "srv_rotated", profile: "beta",
    address: "127.0.0.1:8766", endpoint: "http://127.0.0.1:8766/mcp" };
  const startMCPServer = vi.fn().mockResolvedValue(rotated);
  showServer(vi.fn().mockResolvedValue({ servers: [current] }), {
    listProfiles: vi.fn().mockResolvedValue({ profiles: [
      { profileId: "p_alpha", slug: "alpha", displayName: "Alpha", mode: "hybrid", tools: [] },
      { profileId: "p_beta", slug: "beta", displayName: "Beta", mode: "hybrid", tools: [] },
    ] }),
    startMCPServer,
  });
  expect(await screen.findByText(current.endpoint)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Start server" }));
  const dialog = within(screen.getByRole("dialog", { name: "Start MCP server" }));
  expect(dialog.queryByText(/automatically tries the next local ports/)).toBeNull();
  fireEvent.click(dialog.getByRole("button", { name: "Help for Local listen address" }));
  expect(await screen.findByText(/automatically tries the next local ports/)).toBeTruthy();
  expect(dialog.getByText("127.0.0.1:8765")).toBeTruthy();
  fireEvent.change(dialog.getByLabelText("Server profile"), { target: { value: "beta" } });
  fireEvent.click(dialog.getByRole("button", { name: "Start server" }));
  await waitFor(() => expect(startMCPServer).toHaveBeenCalledWith({
    profile: "beta", address: "127.0.0.1:8765",
  }));
  expect(await screen.findByText(rotated.endpoint)).toBeTruthy();
  expect(screen.getByText(current.endpoint)).toBeTruthy();
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
    const tunnel = await screen.findByText("OpenAI Test Tunnel");
    expect(tunnel.getAttribute("title")).toContain("Starting");
    await waitFor(() => expect(polling.installed()).toBe(true));

    await polling.tick();
    await polling.tick();
    expect(listMCPServers).toHaveBeenCalledTimes(2);
    expect(tunnel.getAttribute("title")).toContain("Starting");

    await act(async () => { finishRequest?.({ servers: [fixture] }); await Promise.resolve(); });
    await polling.tick();
    expect(tunnel.getAttribute("title")).toContain("Starting");
    expect(screen.queryByText("No running MCP servers")).toBeNull();

    await polling.tick();
    await waitFor(() => expect(tunnel.getAttribute("title")).toContain("Ready"));
    expect(listMCPServers).toHaveBeenCalledTimes(4);
  } finally { polling.restore(); }
});

it("renders user-facing Tunnel phases in Chinese rather than raw English enum strings", async () => {
  setLocale("zh-CN");
  const listMCPServers = vi.fn().mockResolvedValue({ servers: [{ ...fixture, tunnelStatus: "ready" }] });
  showServer(listMCPServers);
  const tunnel = await screen.findByText("OpenAI Test Tunnel");
  expect(tunnel.getAttribute("title")).toContain("就绪");
  expect(screen.queryByText("就绪")).toBeNull();
  expect(screen.queryByText("ready")).toBeNull();
});
