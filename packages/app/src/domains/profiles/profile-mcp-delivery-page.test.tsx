import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MCPServerInstance, MenderClient, Profile } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfileMCPDeliveryPage } from "./profile-mcp-delivery-page";

const alpha: Profile = {
  profileId: "prof_00000000000000000000000000000001",
  slug: "alpha", displayName: "Alpha", mode: "hybrid", tools: [], skills: [],
};
const beta: Profile = {
  profileId: "prof_00000000000000000000000000000002",
  slug: "beta", displayName: "Beta", mode: "hybrid", tools: [], skills: [],
};
const alphaServer: MCPServerInstance = {
  serverId: "srv_alpha", profile: "alpha", transport: "streamable-http",
  address: "127.0.0.1:8765", endpoint: "http://127.0.0.1:8765/mcp",
  status: "running", startedAt: "2026-10-23T00:00:00Z",
};
const betaServer: MCPServerInstance = {
  serverId: "srv_beta", profile: "beta", transport: "streamable-http",
  address: "127.0.0.1:8766", endpoint: "http://127.0.0.1:8766/mcp",
  status: "running", startedAt: "2026-10-23T00:00:00Z",
};

function mount(servers: MCPServerInstance[]) {
  const startMCPServer = vi.fn().mockResolvedValue({ ...alphaServer, serverId: "srv_new", endpoint: "http://127.0.0.1:8770/mcp" });
  const restartMCPServer = vi.fn().mockResolvedValue({ ...alphaServer, serverId: "srv_restarted" });
  const stopMCPServer = vi.fn().mockResolvedValue(undefined);
  const client = {
    listProfiles: vi.fn().mockResolvedValue({ profiles: [alpha, beta] }),
    listMCPServers: vi.fn().mockResolvedValue({ servers }),
    startMCPServer, restartMCPServer, stopMCPServer,
  } as unknown as MenderClient;
  render(<MemoryRouter initialEntries={["/profiles/alpha/deliveries/mcp"]}>
    <TooltipProvider><Routes>
      <Route path="/profiles/:slug/deliveries/mcp" element={<ProfileMCPDeliveryPage client={client} copyText={vi.fn().mockResolvedValue(undefined)} />} />
      <Route path="/profiles/:slug" element={<p>Profile detail</p>} />
    </Routes></TooltipProvider>
  </MemoryRouter>);
  return { startMCPServer, restartMCPServer, stopMCPServer };
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("shows only its Profile endpoints and never offers cross-Profile selection", async () => {
  const { startMCPServer } = mount([alphaServer, betaServer]);
  expect(await screen.findByText(alphaServer.endpoint)).toBeTruthy();
  expect(screen.queryByText(betaServer.endpoint)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Start server" }));
  const dialog = within(screen.getByRole("dialog", { name: "Start MCP server" }));
  expect(dialog.queryByLabelText("Server profile")).toBeNull();
  expect(dialog.getByText("MCP delivery belongs to Profile Alpha.")).toBeTruthy();
  fireEvent.click(dialog.getByRole("button", { name: "Start server" }));
  await waitFor(() => expect(startMCPServer).toHaveBeenCalledWith({ profile: "alpha", address: "127.0.0.1:8765" }));
  expect(await screen.findByText("http://127.0.0.1:8770/mcp")).toBeTruthy();
  expect(screen.queryByText(betaServer.endpoint)).toBeNull();
});

it("only exposes stop/restart actions for instances in that Profile", async () => {
  const { restartMCPServer, stopMCPServer } = mount([alphaServer, betaServer]);
  expect(await screen.findByText(alphaServer.endpoint)).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "Restart" })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "Restart" }));
  await waitFor(() => expect(restartMCPServer).toHaveBeenCalledWith("srv_alpha"));
  expect(restartMCPServer).not.toHaveBeenCalledWith("srv_beta");
  fireEvent.click(screen.getByRole("button", { name: "Stop" }));
  await waitFor(() => expect(stopMCPServer).toHaveBeenCalledWith("srv_restarted"));
  expect(stopMCPServer).not.toHaveBeenCalledWith("srv_beta");
});

it("goes back to its Profile instead of showing a separate Servers inventory", async () => {
  mount([]);
  fireEvent.click(await screen.findByRole("button", { name: "Back to profile" }));
  expect(await screen.findByText("Profile detail")).toBeTruthy();
});
