import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import type { ManagementClient } from "@mender/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { PluginDetailPage, PluginsPage } from "./plugins-page";
import { ToolsPage } from "../tools";

const anySearch = {
  offeringId: "io.mender.integration.anysearch",
  displayName: "AnySearch",
  description: "Search the web and retrieve pages through AnySearch's hosted MCP service.",
  kind: "remote-mcp" as const,
  driverPluginId: "io.mender.provider.mcp",
  transport: "streamable-http" as const,
  endpoint: "https://api.anysearch.com/mcp",
  headers: { "X-Anysearch-Client": "mcp/1.0.0" },
  defaultNamespace: "anysearch",
  authentication: "optional-bearer" as const,
  sourceUrl: "https://github.com/anysearch-ai/anysearch-mcp-server",
};

const mcpPlugin = {
  key: "io.mender.provider.mcp@builtin",
  pluginId: "io.mender.provider.mcp",
  extensionId: "main",
  kind: "provider.tools",
  marketplaceAction: "create.provider",
  displayName: "MCP",
  description: "Model Context Protocol provider",
  packageVersion: "1.0.0",
  sourceKind: "builtin",
  status: "enabled",
  enabled: true,
  capabilities: ["mcp.tools"],
  usageCount: 0,
  configVersion: 1,
  protocolVersion: 1,
};

function RouteProbe() {
  const location = useLocation();
  return <output data-testid="route">{location.pathname + location.search}</output>;
}

function setup(initial: string, includeBuiltin = false) {
  const createProvider = vi.fn().mockResolvedValue({});
  const client = {
    listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: includeBuiltin ? [mcpPlugin] : [] }),
    listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [anySearch] }),
    listProviderPlugins: vi.fn().mockResolvedValue({
      plugins: includeBuiltin ? [{
        pluginId: mcpPlugin.pluginId,
        displayName: "MCP",
        description: mcpPlugin.description,
        version: "1.0.0",
        packageVersion: "1.0.0",
        configVersion: 1,
        enabled: true,
        sourceKind: "builtin",
        configSchema: {},
        capabilities: ["mcp.tools"],
      }] : [], issues: [],
    }),
    listProviders: vi.fn().mockResolvedValue({ providers: [] }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listSandboxes: vi.fn().mockResolvedValue({ sandboxes: [] }),
    createProvider,
  } as unknown as ManagementClient;

  render(<MemoryRouter initialEntries={[initial]}>
    <RouteProbe />
    <Routes>
      <Route path="/plugins" element={<PluginsPage client={client} />} />
      <Route path="/plugins/:pluginKey" element={<PluginDetailPage client={client} />} />
      <Route path="/tools" element={<ToolsPage client={client} />} />
    </Routes>
  </MemoryRouter>);
  return createProvider;
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

describe("Plugin detail connection navigation", () => {
  it("restores the original reviewed integration detail on Cancel without creating a service", async () => {
    const createProvider = setup("/plugins");
    fireEvent.click(await screen.findByRole("link", { name: "View AnySearch" }));
    const originalRoute = screen.getByTestId("route").textContent;
    fireEvent.click(await screen.findByRole("button", { name: "Connect" }));
    const dialog = await screen.findByRole("dialog", { name: "Connect AnySearch" });
    await waitFor(() => expect(screen.getByTestId("route").textContent).toBe("/tools"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.getByTestId("route").textContent).toBe(originalRoute));
    expect(await screen.findByRole("button", { name: "Capabilities" })).toBeTruthy();
    expect(createProvider).not.toHaveBeenCalled();
  });

  it("returns to the original plugin detail after cancelling generic Provider creation", async () => {
    const createProvider = setup("/plugins", true);
    fireEvent.click(await screen.findByRole("link", { name: "View MCP" }));
    const originalRoute = screen.getByTestId("route").textContent;
    fireEvent.click(await screen.findByRole("button", { name: "Add provider" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.getByTestId("route").textContent).toBe(originalRoute));
    expect(await screen.findByRole("button", { name: "Capabilities" })).toBeTruthy();
    expect(createProvider).not.toHaveBeenCalled();
  });

  it("stays on Tool services when a direct creation deep link is cancelled", async () => {
    const createProvider = setup("/tools?create=1&offeringId=io.mender.integration.anysearch");
    const dialog = await screen.findByRole("dialog", { name: "Connect AnySearch" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("heading", { name: "Tool services" })).toBeTruthy();
    expect(screen.getByTestId("route").textContent).toBe("/tools");
    expect(createProvider).not.toHaveBeenCalled();
  });

  it("returns to the plugin on dialog Close without creating any provider", async () => {
    const createProvider = setup("/plugins");
    fireEvent.click(await screen.findByRole("link", { name: "View AnySearch" }));
    const originalRoute = screen.getByTestId("route").textContent;
    fireEvent.click(await screen.findByRole("button", { name: "Connect" }));
    const dialog = await screen.findByRole("dialog", { name: "Connect AnySearch" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.getByTestId("route").textContent).toBe(originalRoute));
    expect(createProvider).not.toHaveBeenCalled();
  });

  it("keeps successful connections in Tools and only creates after explicit submission", async () => {
    const createProvider = setup("/plugins");
    fireEvent.click(await screen.findByRole("link", { name: "View AnySearch" }));
    fireEvent.click(await screen.findByRole("button", { name: "Connect" }));
    const dialog = await screen.findByRole("dialog", { name: "Connect AnySearch" });
    expect(createProvider).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(createProvider).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Connect AnySearch" })).toBeNull());
    expect(screen.getByTestId("route").textContent).toBe("/tools");
  });
});
