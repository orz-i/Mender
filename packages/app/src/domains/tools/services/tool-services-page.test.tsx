import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ManagementClient, Provider, ProviderPlugin } from "@mender/sdk";
import { MemoryRouter } from "react-router";
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

function mount(
  providers: Provider[] = [sampleProvider],
  plugins: ProviderPlugin[] = [samplePlugin],
  overrides: Partial<ManagementClient> = {},
) {
  const client = {
    listProviders: vi.fn().mockResolvedValue({ providers }),
    listProviderPlugins: vi.fn().mockResolvedValue({ plugins, issues: [] }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listSandboxes: vi.fn().mockResolvedValue({ sandboxes: [] }),
    ...overrides,
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
  it("shows a named service row with inline sync/view-tools actions and popover management actions", async () => {
    mount();
    const link=await screen.findByRole("link",{name:"Echo Service"});
    expect(link.getAttribute("href")).toBe("/tools/prov_echo_01");
    const card=link.closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText("5 tools")).toBeTruthy();
    expect(card!.querySelector(".bg-teal")).toBeTruthy();
    expect(within(card!).queryByText("Model Context Protocol")).toBeNull();
    const viewToolsLink = within(card!).getByRole("link", { name: "View tools" });
    expect(viewToolsLink.getAttribute("href")).toBe("/tools/prov_echo_01");
    expect(within(card!).getByRole("button", { name: "Sync Echo Service" })).toBeTruthy();
    expect(screen.queryByRole("navigation", { name: "Tool services pagination" })).toBeNull();
    expect(within(card!).queryByRole("button",{name:"View Echo Service"})).toBeNull();
    fireEvent.click(within(card!).getByRole("button",{name:"Actions: Echo Service"}));
    expect(await screen.findByRole("button",{name:"View Echo Service"})).toBeTruthy();
    expect(screen.getByRole("button",{name:"Edit Echo Service"})).toBeTruthy();
    expect(screen.getByRole("button",{name:"Delete Echo Service"})).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Show internal providers"})).toBeNull();
  });

  it("confirms provider deletion via an in-app dialog instead of window.confirm", async () => {
    const deleteProvider = vi.fn().mockResolvedValue({});
    const confirmSpy = vi.spyOn(window, "confirm").mockImplementation(() => {
      throw new Error("native confirm must not be called");
    });
    try {
      mount([sampleProvider], [samplePlugin], { deleteProvider });
      const card = (await screen.findByRole("link", { name: "Echo Service" }))
        .closest<HTMLElement>('[role="listitem"]');
      expect(card).toBeTruthy();

      fireEvent.click(within(card!).getByRole("button", { name: "Actions: Echo Service" }));
      fireEvent.click(await screen.findByRole("button", { name: "Delete Echo Service" }));

      const dialog = await screen.findByRole("dialog", { name: "Delete Echo Service" });
      expect(within(dialog).getByText('Delete provider "echo_service"? Profile references will block deletion.')).toBeTruthy();
      fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
      await waitFor(() => expect(screen.queryByRole("dialog", { name: "Delete Echo Service" })).toBeNull());
      expect(deleteProvider).not.toHaveBeenCalled();

      fireEvent.click(within(card!).getByRole("button", { name: "Actions: Echo Service" }));
      fireEvent.click(await screen.findByRole("button", { name: "Delete Echo Service" }));
      const confirmDialog = await screen.findByRole("dialog", { name: "Delete Echo Service" });
      fireEvent.click(within(confirmDialog).getByRole("button", { name: "Delete" }));

      await waitFor(() => expect(deleteProvider).toHaveBeenCalledWith("prov_echo_01"));
      expect(confirmSpy).not.toHaveBeenCalled();
    } finally {
      confirmSpy.mockRestore();
    }
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
    expect(within(card!).getByRole("link", { name: "View tools" })).toBeTruthy();
    expect(within(card!).getByRole("button", { name: "Actions: Echo Service" })).toBeTruthy();
  });

  it("routes zero-tool services to details, while retaining explicit sync and edit", async () => {
    mount([{ ...sampleProvider, description: "", toolCount: 0 }]);
    const card = (await screen.findByRole("link", { name: "Echo Service" }))
      .closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText("0 tools")).toBeTruthy();
    expect(card!.querySelector(".bg-teal")).toBeNull();
    expect(within(card!).getByText("No tools have been discovered for this service yet.")).toBeTruthy();
    expect(within(card!).getByRole("button", { name: "Sync Echo Service" })).toBeTruthy();
    fireEvent.click(within(card!).getByRole("button", { name: "Actions: Echo Service" }));
    expect(await screen.findByRole("button", { name: "Edit Echo Service" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Delete Echo Service" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "View Echo Service" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "View Echo Service" }));
    expect(await screen.findByRole("dialog", { name: "Provider details" })).toBeTruthy();
  });

  it("keeps the zero-tool detail exit translated in Chinese", async () => {
    setLocale("zh-CN");
    mount([{ ...sampleProvider, description: "", toolCount: 0 }]);
    expect(await screen.findByRole("link", { name: "Echo Service" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "操作: Echo Service" }));
    fireEvent.click(screen.getByRole("button", { name: "查看 Echo Service" }));
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

describe("External Provider descriptions", () => {
  it("preserves configured display and content even when it resembles historic internal template", async () => {
    const original="Managed Agent Delegation tools for Grok";
    mount([{...sampleProvider,description:original,toolSourceKind:"provider"}]);
    const card=(await screen.findByRole("link",{name:"Echo Service"})).closest<HTMLElement>('[role="listitem"]');
    expect(card).toBeTruthy();
    expect(within(card!).getByText(original)).toBeTruthy();
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
