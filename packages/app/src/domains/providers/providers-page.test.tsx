import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient, Provider, ProviderPlugin } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProvidersPage } from "./providers-page";

const sampleProvider: Provider = {
  providerId: "prov_echo_01",
  namespace: "echo_service",
  displayName: "Echo Service",
  description: "Echo service description",
  pluginId: "mcp",
  pluginDisplayName: "Model Context Protocol",
  configVersion: 1,
  transport: "http" as const,
  endpoint: "https://api.echo.com/mcp",
  toolCount: 5,
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
};

const samplePlugin: ProviderPlugin = {
  pluginId: "mcp",
  displayName: "Model Context Protocol",
  description: "Model Context Protocol",
  version: "1.0.0",
  packageVersion: "1.0.0",
  configVersion: 1,
  enabled: true,
  sourceKind: "builtin",
  configSchema: {},
  capabilities: ["mcp.tools"],
};

function mount(providers: Provider[] = [sampleProvider], plugins: ProviderPlugin[] = [samplePlugin]) {
  const client = {
    listProviders: vi.fn().mockResolvedValue({ providers }),
    listProviderPlugins: vi.fn().mockResolvedValue({ plugins, issues: [] }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listSandboxes: vi.fn().mockResolvedValue({ sandboxes: [] }),
  } as unknown as MenderClient;

  render(
    <MemoryRouter>
      <ProvidersPage client={client} />
    </MemoryRouter>,
  );
  return { client };
}

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("Tool services responsive cards", () => {
  it("shows one named service card with a Tool drilldown action", async () => {
    mount();
    const link=await screen.findByRole("link",{name:"Echo Service"});
    expect(link.getAttribute("href")).toBe("/tools/prov_echo_01");
    const card=link.closest<HTMLElement>('[data-slot="card"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText("5 tools")).toBeTruthy();
    expect(within(card!).queryByText("Model Context Protocol")).toBeNull();
    expect(screen.getByRole("button",{name:"View tools"})).toBeTruthy();
    expect(screen.queryByRole("navigation", { name: "Tool services pagination" })).toBeNull();
    expect(within(card!).queryByRole("button",{name:"View Echo Service"})).toBeNull();
    fireEvent.click(within(card!).getByRole("button",{name:"Actions: Echo Service"}));
    expect(await screen.findByRole("button",{name:"View Echo Service"})).toBeTruthy();
    expect(screen.getByRole("button",{name:"Edit Echo Service"})).toBeTruthy();
    expect(screen.getByRole("button",{name:"Sync Echo Service"})).toBeTruthy();
    expect(screen.getByRole("button",{name:"Delete Echo Service"})).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Show internal providers"})).toBeNull();
  });
});

describe("Tool service error state", () => {
  it("shows a failed request and retry without falsely claiming there are no services", async () => {
    const listProviders = vi.fn()
      .mockRejectedValueOnce(new Error("Provider list unavailable"))
      .mockResolvedValue({ providers: [] });
    const client = {
      listProviders,
      listProviderPlugins: vi.fn().mockResolvedValue({ plugins: [], issues: [] }),
      listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    } as unknown as MenderClient;

    render(<MemoryRouter><ProvidersPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Provider list unavailable");
    expect(screen.queryByText("No tool services")).toBeNull();
    fireEvent.click(within(alert).getByRole("button", { name: "Refresh" }));
    await screen.findByText("No tool services");
    await waitFor(() => expect(listProviders).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("resource-owned Tool service navigation", () => {
  it.each([
    {
      kind: "agent" as const,
      pluginId: "io.mender.provider.agent-delegation",
      ownerId: "agent_grok",
      ownerName: "Grok - executor",
      visibleName: "Grok - executor",
      destination: "Agent owner details",
    },
    {
      kind: "sandbox" as const,
      pluginId: "io.mender.provider.workspace",
      ownerId: "sbx_rocky",
      ownerName: "Rocky",
      visibleName: "Rocky · Workspace",
      destination: "Sandbox owner details",
    },
    {
      kind: "sandbox" as const,
      pluginId: "io.mender.provider.sandbox-jobs",
      ownerId: "sbx_rocky",
      ownerName: "Rocky",
      visibleName: "Rocky · Jobs",
      destination: "Sandbox owner details",
    },
  ])("opens $kind owner for $visibleName without generic Provider mutation controls", async ({
    kind, pluginId, ownerId, ownerName, visibleName, destination,
  }) => {
    const source: Provider = {
      ...sampleProvider,
      providerId: "prov_managed_01",
      displayName: "Internal Facade",
      pluginId,
      toolSourceKind: kind,
      toolSourceId: ownerId,
      toolSourceName: ownerName,
    };
    const client = {
      listProviders: vi.fn().mockResolvedValue({ providers: [source] }),
      listProviderPlugins: vi.fn().mockResolvedValue({ plugins: [], issues: [] }),
      listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    } as unknown as MenderClient;

    render(
      <MemoryRouter initialEntries={["/tools"]}>
        <Routes>
          <Route path="/tools" element={<ProvidersPage client={client} />} />
          <Route path="/agents/:agentId" element={<div>Agent owner details</div>} />
          <Route path="/sandboxes/:sandboxId" element={<div>Sandbox owner details</div>} />
        </Routes>
      </MemoryRouter>,
    );
    const link = await screen.findByRole("link", { name: visibleName });
    const card = link.closest<HTMLElement>('[data-slot="card"]');
    expect(card).toBeTruthy();
    expect(within(card!).queryByRole("button", { name: "Edit " + visibleName })).toBeNull();
    expect(within(card!).queryByRole("button", { name: "Delete " + visibleName })).toBeNull();
    fireEvent.click(within(card!).getByRole("button", { name: "Manage" }));
    expect(await screen.findByText(destination)).toBeTruthy();
  });
});
