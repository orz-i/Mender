import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ManagementClient } from "@mender/sdk";
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
    } as unknown as ManagementClient;
    render(
      <MemoryRouter initialEntries={["/plugins"]}>
        <Routes>
          <Route path="/plugins" element={<PluginsPage client={client} />} />
          <Route path="/plugins/:pluginKey" element={<PluginDetailPage client={client} />} />
        </Routes>
      </MemoryRouter>,
    );

    const configureLink = await screen.findByRole("link", { name: "Configure Example Search" });
    expect(configureLink.getAttribute("href")).toContain("/tools?create=1");
    expect(within(configureLink).getByText("Example Search")).toBeTruthy();
    const cardLink = screen.getByRole("link", { name: "View Example Search" });
    expect(screen.queryByRole("navigation", { name: "Plugin Marketplace pagination" })).toBeNull();
    expect(screen.getByRole("region", { name: "Provider plugin" })).toBeTruthy();
    expect(within(cardLink).queryByText("Provider plugin")).toBeNull();
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

describe("Marketplace direct setup path", () => {
  it("opens a vetted provider creation route directly while preserving optional package details", async () => {
    const client = {
      listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: [{
        key: "io.mender.provider.mcp@builtin", pluginId: "io.mender.provider.mcp",
        extensionId: "main", kind: "provider.tools", marketplaceAction: "create.provider",
        displayName: "MCP", description: "Model Context Protocol provider", packageVersion: "1.0.0",
        sourceKind: "builtin", status: "enabled", enabled: true, capabilities: ["mcp.tools"],
        usageCount: 0, configVersion: 1, protocolVersion: 1,
      }] }),
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
    } as unknown as ManagementClient;
    render(<MemoryRouter initialEntries={["/plugins"]}><Routes>
      <Route path="/plugins" element={<PluginsPage client={client} />} />
      <Route path="/tools" element={<p>Provider setup destination</p>} />
    </Routes></MemoryRouter>);
    const create = await screen.findByRole("link", { name: "Configure MCP" });
    expect(create.getAttribute("href")).toContain("/tools?create=1");
    expect(create.getAttribute("href")).toContain("pluginId=io.mender.provider.mcp");
    expect(screen.getByRole("link", { name: "View MCP" })).toBeTruthy();
    fireEvent.click(create);
    expect(screen.getByText("Provider setup destination")).toBeTruthy();
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
    } as unknown as ManagementClient;
    render(<MemoryRouter initialEntries={["/plugins"]}><Routes>
      <Route path="/plugins" element={<PluginsPage client={client} />} />
      <Route path="/plugins/:pluginKey" element={<PluginDetailPage client={client} />} />
    </Routes></MemoryRouter>);
    const configure = await screen.findByRole("link", { name: "Configure MCP" });
    expect(within(configure).getByText(raw)).toBeTruthy();
    expect(screen.getByRole("link", { name: "View MCP" })).toBeTruthy();

    act(() => setLocale("zh-CN"));
    expect(within(configure).getByText("通过 MCP 协议接入外部工具来源")).toBeTruthy();
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
    } as unknown as ManagementClient;
    setLocale("zh-CN");
    render(<MemoryRouter><PluginsPage client={client} /></MemoryRouter>);
    const card = await screen.findByRole("link", { name: "配置 AnySearch" });
    expect(within(card).getByText("通过 AnySearch 托管的 MCP 服务搜索网页并获取页面内容")).toBeTruthy();
    expect(screen.getByRole("link", { name: "查看 AnySearch" })).toBeTruthy();
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
    } as unknown as ManagementClient;

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

describe("Registry online installation and upgrade", () => {
  it("renders 1-click install button in discover view with progress state", async () => {
    let installed = false;
    const installRegistryPlugin = vi.fn().mockImplementation(() => {
      installed = true;
      return Promise.resolve({
        installation: {
          installationId: "pinst_reg_001",
          pluginId: "dev.example.websearch",
          packageVersion: "1.0.0",
          sourceKind: "registry",
        },
      });
    });
    const listPluginCatalogue = vi.fn().mockImplementation(() => {
      if (!installed) {
        return Promise.resolve({
          plugins: [
            {
              key: "dev.example.websearch@registry",
              pluginId: "dev.example.websearch",
              extensionId: "main",
              kind: "provider.tools",
              marketplaceAction: "none",
              displayName: "WebSearch",
              description: "Web search online provider",
              packageVersion: "1.0.0",
              sourceKind: "registry",
              status: "available",
              enabled: false,
              capabilities: ["tool.search"],
              usageCount: 0,
            },
          ],
        });
      }
      return Promise.resolve({
        plugins: [
          {
            key: "dev.example.websearch@pinst_reg_001",
            pluginId: "dev.example.websearch",
            extensionId: "main",
            kind: "provider.tools",
            marketplaceAction: "create.provider",
            displayName: "WebSearch",
            description: "Web search online provider",
            packageVersion: "1.0.0",
            sourceKind: "registry",
            status: "enabled",
            enabled: true,
            installationId: "pinst_reg_001",
            capabilities: ["tool.search"],
            usageCount: 0,
          },
        ],
      });
    });

    const client = {
      listPluginCatalogue,
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
      listRegistryUpdates: vi.fn().mockResolvedValue({ updates: [] }),
      installRegistryPlugin,
    } as unknown as ManagementClient;

    render(
      <MemoryRouter initialEntries={["/plugins?view=discover"]}>
        <PluginsPage client={client} />
      </MemoryRouter>,
    );

    const installButton = await screen.findByRole("button", { name: "Install" });
    expect(installButton).toBeTruthy();
    expect(screen.getByText("WebSearch")).toBeTruthy();

    fireEvent.click(installButton);
    expect(installRegistryPlugin).toHaveBeenCalledWith({
      pluginId: "dev.example.websearch",
      version: "1.0.0",
    });
  });

  it("renders update available badge and upgrade button when usageCount is 0", async () => {
    let upgraded = false;
    const upgradeRegistryPlugin = vi.fn().mockImplementation(() => {
      upgraded = true;
      return Promise.resolve({
        installation: {
          installationId: "pinst_reg_001",
          packageVersion: "1.1.0",
        },
      });
    });
    const listPluginCatalogue = vi.fn().mockImplementation(() => {
      return Promise.resolve({
        plugins: [
          {
            key: "dev.example.websearch@pinst_reg_001",
            pluginId: "dev.example.websearch",
            extensionId: "main",
            kind: "provider.tools",
            marketplaceAction: "create.provider",
            displayName: "WebSearch",
            description: "Web search online provider",
            packageVersion: upgraded ? "1.1.0" : "1.0.0",
            sourceKind: "registry",
            status: "enabled",
            enabled: true,
            installationId: "pinst_reg_001",
            capabilities: ["tool.search"],
            usageCount: 0,
          },
        ],
      });
    });
    const listRegistryUpdates = vi.fn().mockImplementation(() => {
      if (upgraded) return Promise.resolve({ updates: [] });
      return Promise.resolve({
        updates: [
          {
            installationId: "pinst_reg_001",
            pluginId: "dev.example.websearch",
            currentVersion: "1.0.0",
            targetVersion: "1.1.0",
          },
        ],
      });
    });

    const client = {
      listPluginCatalogue,
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
      listRegistryUpdates,
      upgradeRegistryPlugin,
    } as unknown as ManagementClient;

    render(
      <MemoryRouter initialEntries={["/plugins?view=installed"]}>
        <PluginsPage client={client} />
      </MemoryRouter>,
    );

    expect(await screen.findByText("Update available")).toBeTruthy();
    const upgradeButton = screen.getByRole("button", { name: "Upgrade" });
    expect(upgradeButton).toBeTruthy();

    fireEvent.click(upgradeButton);
    expect(upgradeRegistryPlugin).toHaveBeenCalledWith({
      installationId: "pinst_reg_001",
      version: "1.1.0",
    });
  });

  it("blocks in-place upgrade when usageCount > 0 and indicates referenced resources", async () => {
    const client = {
      listPluginCatalogue: vi.fn().mockResolvedValue({
        plugins: [
          {
            key: "dev.example.websearch@pinst_reg_001",
            pluginId: "dev.example.websearch",
            extensionId: "main",
            kind: "provider.tools",
            marketplaceAction: "create.provider",
            displayName: "WebSearch",
            description: "Web search online provider",
            packageVersion: "1.0.0",
            sourceKind: "registry",
            status: "enabled",
            enabled: true,
            installationId: "pinst_reg_001",
            capabilities: ["tool.search"],
            usageCount: 3,
          },
        ],
      }),
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
      listRegistryUpdates: vi.fn().mockResolvedValue({
        updates: [
          {
            installationId: "pinst_reg_001",
            pluginId: "dev.example.websearch",
            currentVersion: "1.0.0",
            targetVersion: "1.1.0",
          },
        ],
      }),
    } as unknown as ManagementClient;

    render(
      <MemoryRouter initialEntries={["/plugins?view=installed"]}>
        <PluginsPage client={client} />
      </MemoryRouter>,
    );

    expect(await screen.findByText("Update available")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Upgrade" })).toBeNull();
    const inUseBadge = screen.getByText("In use");
    expect(inUseBadge).toBeTruthy();
    expect(inUseBadge.getAttribute("title")).toBe("Referenced resources block in-place upgrade");
  });

  it("localizes registry offering, install, and upgrade states in zh-CN", async () => {
    setLocale("zh-CN");
    const client = {
      listPluginCatalogue: vi.fn().mockResolvedValue({
        plugins: [
          {
            key: "dev.example.tool@registry",
            pluginId: "dev.example.tool",
            extensionId: "main",
            kind: "provider.tools",
            marketplaceAction: "none",
            displayName: "Tool",
            description: "A registry tool",
            packageVersion: "1.0.0",
            sourceKind: "registry",
            status: "available",
            enabled: false,
            capabilities: [],
            usageCount: 0,
          },
        ],
      }),
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
      listRegistryUpdates: vi.fn().mockResolvedValue({ updates: [] }),
    } as unknown as ManagementClient;

    render(
      <MemoryRouter initialEntries={["/plugins?view=discover"]}>
        <PluginsPage client={client} />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("button", { name: "安装" })).toBeTruthy();
  });
});
