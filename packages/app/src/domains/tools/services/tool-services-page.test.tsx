import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ManagementClient, Provider, ProviderPlugin } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../../i18n/i18n";
import { ToolsPage } from "./tool-services-page";

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
  } as unknown as ManagementClient;

  render(
    <MemoryRouter>
      <ToolsPage client={client} />
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

describe("Tool services resource rows", () => {
  it("shows a named service row with real management actions", async () => {
    mount();
    const link=await screen.findByRole("link",{name:"Echo Service"});
    expect(link.getAttribute("href")).toBe("/tools/prov_echo_01");
    const card=link.closest<HTMLElement>('[role="listitem"]');
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

describe("Tool service low-noise empty and active states", () => {
  it("does not repeat generic filler for described-by-name sources with discovered tools", async () => {
    mount([{ ...sampleProvider, description: "" }]);
    const card = (await screen.findByRole("link", { name: "Echo Service" }))
      .closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(card!.querySelector('[data-slot="card-description"]')).toBeNull();
    expect(within(card!).getByText("5 tools")).toBeTruthy();
    expect(within(card!).getByRole("button", { name: "View tools" })).toBeTruthy();
    expect(within(card!).getByRole("button", { name: "Actions: Echo Service" })).toBeTruthy();
  });

  it("routes zero-tool services to details, while retaining explicit sync and edit", async () => {
    mount([{ ...sampleProvider, description: "", toolCount: 0 }]);
    const card = (await screen.findByRole("link", { name: "Echo Service" }))
      .closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText("0 tools")).toBeTruthy();
    expect(within(card!).getByText("No tools have been discovered for this service yet.")).toBeTruthy();
    expect(within(card!).queryByRole("button", { name: "View tools" })).toBeNull();
    fireEvent.click(within(card!).getByRole("button", { name: "Actions: Echo Service" }));
    expect(await screen.findByRole("button", { name: "Sync Echo Service" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit Echo Service" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Delete Echo Service" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "View Echo Service" })).toBeNull();
    fireEvent.click(within(card!).getByRole("button", { name: "Details" }));
    expect(await screen.findByRole("dialog", { name: "Provider details" })).toBeTruthy();
  });

  it("keeps the zero-tool detail exit translated in Chinese", async () => {
    setLocale("zh-CN");
    mount([{ ...sampleProvider, description: "", toolCount: 0 }]);
    expect(await screen.findByRole("link", { name: "Echo Service" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "详情" }));
    const dialog = await screen.findByRole("dialog", { name: "提供方详情" });
    fireEvent.click(within(dialog).getByRole("button", { name: "关闭" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "提供方详情" })).toBeNull());
  });

  it("does not replace user-provided source descriptions with a generated sentence", async () => {
    mount([{ ...sampleProvider, description: "Document lookup service", toolCount: 0 }]);
    const card = (await screen.findByRole("link", { name: "Echo Service" }))
      .closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText("Document lookup service")).toBeTruthy();
    expect(within(card!).queryByText("No tools have been discovered for this service yet.")).toBeNull();
  });
});

describe("Owned Tool service localized summary", () => {
  it("shows accurate host-generated text and searches both Chinese and original English", async () => {
    const raw = "Managed Agent Delegation tools for Grok";
    const owned: Provider = {
      ...sampleProvider,
      providerId: "prov_agent_grok",
      displayName: "Grok service",
      description: raw,
      toolCount: 4,
      pluginId: "io.mender.provider.agent-delegation",
      toolSourceKind: "agent",
      toolSourceId: "agent_grok",
      toolSourceName: "Grok",
    };
    mount([owned], []);
    const card = (await screen.findByRole("link", { name: "Grok" }))
      .closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText(raw)).toBeTruthy();

    act(() => setLocale("zh-CN"));
    expect(within(card!).getByText("Agent「Grok」的受管委派工具")).toBeTruthy();
    const search = screen.getByRole("textbox", { name: "搜索服务" });
    fireEvent.change(search, { target: { value: "受管委派工具" } });
    expect(screen.getByRole("link", { name: "Grok" })).toBeTruthy();
    fireEvent.change(search, { target: { value: raw } });
    expect(screen.getByRole("link", { name: "Grok" })).toBeTruthy();

    act(() => setLocale("en"));
    expect(within(card!).getByText(raw)).toBeTruthy();
    expect(owned.description).toBe(raw);
  });

  it("keeps arbitrary Provider descriptions unchanged, even with matching English templates", async () => {
    const original = "Managed Agent Delegation tools for Grok";
    const external: Provider = {
      ...sampleProvider, providerId: "prov_external", description: original,
      toolSourceName: "Grok", toolSourceKind: "provider",
    };
    setLocale("zh-CN");
    mount([external]);
    const card = (await screen.findByRole("link", { name: "Grok" }))
      .closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText(original)).toBeTruthy();
    expect(within(card!).queryByText("Agent「Grok」的受管委派工具")).toBeNull();
  });
});

describe("Tool service incomplete display metadata", () => {
  it("does not blank the page when optional source descriptions and plugin issue metadata are missing", async () => {
    const source = { ...sampleProvider, description: undefined } as unknown as Provider;
    const client = {
      listProviders: vi.fn().mockResolvedValue({ providers: [source] }),
      listProviderPlugins: vi.fn().mockResolvedValue({ plugins: [] }),
      listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    } as unknown as ManagementClient;
    render(<MemoryRouter><ToolsPage client={client} /></MemoryRouter>);
    const list = await screen.findByRole("list", { name: "Tool services" });
    expect(within(list).getByRole("link", { name: "Echo Service" })).toBeTruthy();
    expect(within(list).getByText("5 tools")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
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
    } as unknown as ManagementClient;

    render(<MemoryRouter><ToolsPage client={client} /></MemoryRouter>);

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
  it("uses Manage for an Agent-owned zero-tool source without exposing generic mutation actions", async () => {
    const source: Provider = {
      ...sampleProvider,
      providerId: "prov_empty_agent",
      description: "",
      toolCount: 0,
      toolSourceKind: "agent",
      toolSourceId: "agent_grok",
      toolSourceName: "Grok - executor",
      pluginId: "io.mender.provider.agent-delegation",
    };
    const client = {
      listProviders: vi.fn().mockResolvedValue({ providers: [source] }),
      listProviderPlugins: vi.fn().mockResolvedValue({ plugins: [], issues: [] }),
      listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    } as unknown as ManagementClient;
    render(<MemoryRouter initialEntries={["/tools"]}><Routes>
      <Route path="/tools" element={<ToolsPage client={client} />} />
      <Route path="/agents/:agentId" element={<div>Agent owner details</div>} />
    </Routes></MemoryRouter>);
    const card = (await screen.findByRole("link", { name: "Grok - executor" }))
      .closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText("No tools have been discovered for this service yet.")).toBeTruthy();
    expect(within(card!).queryByRole("button", { name: "View tools" })).toBeNull();
    expect(within(card!).queryByRole("button", { name: /Actions/ })).toBeNull();
    expect(within(card!).getByRole("link", { name: "Grok - executor" }).getAttribute("href"))
      .toBe("/tools/prov_empty_agent");
    fireEvent.click(within(card!).getByRole("button", { name: "Manage" }));
    expect(await screen.findByText("Agent owner details")).toBeTruthy();
  });
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
    } as unknown as ManagementClient;

    render(
      <MemoryRouter initialEntries={["/tools"]}>
        <Routes>
          <Route path="/tools" element={<ToolsPage client={client} />} />
          <Route path="/agents/:agentId" element={<div>Agent owner details</div>} />
          <Route path="/sandboxes/:sandboxId" element={<div>Sandbox owner details</div>} />
        </Routes>
      </MemoryRouter>,
    );
    const link = await screen.findByRole("link", { name: visibleName });
    const card = link.closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).queryByRole("button", { name: "Edit " + visibleName })).toBeNull();
    expect(within(card!).queryByRole("button", { name: "Delete " + visibleName })).toBeNull();
    fireEvent.click(within(card!).getByRole("button", { name: "Manage" }));
    expect(await screen.findByText(destination)).toBeTruthy();
  });
});
