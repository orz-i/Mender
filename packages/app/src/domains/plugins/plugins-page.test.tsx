import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { PluginDetailPage, PluginsPage } from "./plugins-page";

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("PluginsPage card density", () => {
  it("shows just the summary while preserving capabilities and version in detail", async () => {
    const client = {
      listPluginCatalogue: vi.fn().mockResolvedValue({
        plugins: [{
          key: "dev.example.test@builtin",
          pluginId: "dev.example.test",
          extensionId: "main",
          kind: "provider.tools",
          marketplaceAction: "create.provider",
          displayName: "Example Search",
          description: "Search and research services",
          packageVersion: "2.1.0",
          sourceKind: "builtin",
          status: "enabled",
          enabled: true,
          capabilities: ["tool.search"],
          usageCount: 0,
          configVersion: 1,
          protocolVersion: 1,
        }],
      }),
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
    } as unknown as MenderClient;
    render(
      <MemoryRouter initialEntries={["/plugins"]}>
        <Routes>
          <Route path="/plugins" element={<PluginsPage client={client} />} />
          <Route path="/plugins/:pluginKey" element={<PluginDetailPage client={client} />} />
        </Routes>
      </MemoryRouter>,
    );

    const cardLink = await screen.findByRole("link", { name: "View Example Search" });
    expect(screen.queryByRole("navigation", { name: "Plugin Marketplace pagination" })).toBeNull();
    expect(within(cardLink).getAllByText("Provider plugin").length).toBe(1);
    expect(within(cardLink).queryByText("tool.search")).toBeNull();
    expect(within(cardLink).queryByText("2.1.0")).toBeNull();
    fireEvent.click(cardLink);
    expect(await screen.findByRole("button", { name: "Capabilities" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Capabilities" }));
    expect(await screen.findByText("tool.search")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Overview" }));
    expect(await screen.findByText("2.1.0")).toBeTruthy();
  });
});

describe("Marketplace host description localization", () => {
  it("keeps builtin cards, source-language search and details consistent across locale changes", async () => {
    const raw = "Model Context Protocol provider";
    const client = {
      listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: [{
        key: "io.mender.provider.mcp@builtin",
        pluginId: "io.mender.provider.mcp",
        extensionId: "main",
        kind: "provider.tools",
        marketplaceAction: "create.provider",
        displayName: "MCP",
        description: raw,
        packageVersion: "1.0.0",
        sourceKind: "builtin",
        status: "enabled",
        enabled: true,
        capabilities: ["mcp.tools"],
        usageCount: 0,
        configVersion: 1,
        protocolVersion: 1,
      }] }),
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
    } as unknown as MenderClient;
    render(<MemoryRouter initialEntries={["/plugins"]}><Routes>
      <Route path="/plugins" element={<PluginsPage client={client} />} />
      <Route path="/plugins/:pluginKey" element={<PluginDetailPage client={client} />} />
    </Routes></MemoryRouter>);
    const link = await screen.findByRole("link", { name: "View MCP" });
    expect(within(link).getByText(raw)).toBeTruthy();

    act(() => setLocale("zh-CN"));
    expect(within(link).getByText("通过 MCP 协议接入外部工具来源")).toBeTruthy();
    const search = screen.getByRole("textbox", { name: "搜索插件" });
    fireEvent.change(search, { target: { value: "接入外部工具来源" } });
    expect(screen.getByRole("link", { name: "查看 MCP" })).toBeTruthy();
    fireEvent.change(search, { target: { value: raw } });
    expect(screen.getByRole("link", { name: "查看 MCP" })).toBeTruthy();

    fireEvent.click(screen.getByRole("link", { name: "查看 MCP" }));
    expect(await screen.findByText("通过 MCP 协议接入外部工具来源")).toBeTruthy();
    act(() => setLocale("en"));
    expect(await screen.findByText(raw)).toBeTruthy();
  });

  it("localizes reviewed integration description while keeping the external offering immutable", async () => {
    const offering = {
      offeringId: "io.mender.integration.anysearch",
      displayName: "AnySearch",
      description: "Search the web and retrieve pages through AnySearch's hosted MCP service.",
      kind: "remote-mcp", driverPluginId: "io.mender.provider.mcp",
      transport: "streamable-http", endpoint: "https://api.anysearch.com/mcp",
      headers: { "X-Anysearch-Client": "mcp/1.0.0" },
      defaultNamespace: "anysearch", authentication: "optional-bearer",
      sourceUrl: "https://github.com/anysearch-ai/anysearch-mcp-server",
    };
    const client = {
      listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: [] }),
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [offering] }),
    } as unknown as MenderClient;
    setLocale("zh-CN");
    render(<MemoryRouter><PluginsPage client={client} /></MemoryRouter>);
    const card = await screen.findByRole("link", { name: "查看 AnySearch" });
    expect(within(card).getByText("通过 AnySearch 托管的 MCP 服务搜索网页并获取页面内容")).toBeTruthy();
    expect(offering.description).toBe("Search the web and retrieve pages through AnySearch's hosted MCP service.");
  });
});

describe("PluginsPage initial load", () => {
  it("does not confuse request errors with an empty marketplace and offers retry", async () => {
    const listPluginCatalogue = vi.fn()
      .mockRejectedValueOnce(new Error("Catalogue unavailable"))
      .mockResolvedValue({ plugins: [] });
    const client = {
      listPluginCatalogue,
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
    } as unknown as MenderClient;

    render(<MemoryRouter><PluginsPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Catalogue unavailable");
    expect(screen.queryByText("No matching plugins")).toBeNull();

    fireEvent.click(within(alert).getByRole("button", { name: "Refresh" }));
    await screen.findByText("No matching plugins");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(listPluginCatalogue).toHaveBeenCalledTimes(2);
  });
});
