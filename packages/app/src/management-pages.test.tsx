import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type {
  MenderClient,
  PolicyRuleInput,
  ProviderPluginInstallation,
} from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MenderApp } from "./mender-app";

afterEach(cleanup);

function expectAnchoredDialog(title: string) {
  const dialog = screen.getByRole("dialog", { name: title });
  expect(dialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
  expect(dialog.className).toContain("overflow-hidden");
  expect(dialog.querySelector('[data-slot="dialog-body"]')?.className).toContain("overflow-y-auto");
  expect(dialog.querySelector('[data-slot="dialog-footer"]')).toBeTruthy();
}

describe("management pages", () => {
  it("keeps Provider creation out of the primary list surface", async () => {
    const provider = {
      providerId: "prov_1",
      namespace: "demo",
      displayName: "Demo",
      description: "",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
      transport: "http" as const,
      toolCount: 1,
      endpoint: "https://example.test/mcp",
    };
    const credential = {
      reference: "credential://demo/work",
      namespace: "demo",
      name: "work",
      displayName: "Demo work",
      source: "environment" as const,
      environmentVariable: "DEMO_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      createProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    expect(screen.queryByText("Remote endpoint")).toBeNull();
    expect(screen.queryByText("Local command")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));
    const chooser = screen.getByRole("dialog", {
      name: "Choose Provider plugin",
    });
    expect(within(chooser).queryByText("Remote endpoint")).toBeNull();
    expect(within(chooser).queryByText("Local command")).toBeNull();
    fireEvent.click(
      within(chooser).getByRole("button", { name: "Add MCP provider" }),
    );

    const createDialog = screen.getByRole("dialog", {
      name: "Add MCP provider",
    });
    expect(within(createDialog).getByText("Remote endpoint")).toBeTruthy();
    expect(within(createDialog).getByText("Local command")).toBeTruthy();
    expect(
      within(createDialog).queryByRole("button", {
        name: "Add Model Tools provider",
      }),
    ).toBeNull();

    fireEvent.click(
      within(createDialog).getByRole("button", { name: "Back to plugins" }),
    );
    const reopenedChooser = await screen.findByRole("dialog", {
      name: "Choose Provider plugin",
    });
    expect(within(reopenedChooser).queryByText("Remote endpoint")).toBeNull();
    fireEvent.click(
      within(reopenedChooser).getByRole("button", {
        name: "Add MCP provider",
      }),
    );

    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "demo" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Demo" },
    });
    fireEvent.change(screen.getByLabelText("Endpoint"), {
      target: { value: "https://example.test/mcp" },
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Configure authentication" }),
    );
    expect(await screen.findByText("Provider authentication")).toBeTruthy();
    expectAnchoredDialog("Provider authentication");
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.click(
      screen.getByRole("button", { name: "Add credential header" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Use authentication" }),
    );

    await waitFor(() => {
      expect(screen.getAllByRole("dialog")).toHaveLength(1);
    });
    expect(screen.getByLabelText<HTMLInputElement>("Namespace").value).toBe(
      "demo",
    );
    expect(screen.getByLabelText<HTMLInputElement>("Endpoint").value).toBe(
      "https://example.test/mcp",
    );

    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));

    await waitFor(() => {
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "demo",
        displayName: "Demo",
        pluginId: "io.mender.provider.mcp",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://example.test/mcp",
            credentialHeaders: [
              {
                name: "Authorization",
                scheme: "Bearer",
                credentialRef: "credential://demo/work",
              },
            ],
          },
        },
      });
    });
    expect(await screen.findByText("Demo")).toBeTruthy();
    expect(screen.queryByText("demo")).toBeNull();
  });

  it("routes Provider plugin degradation to Marketplace without hiding healthy plugins", async () => {
    renderApp("/providers", {
      listProviderPlugins: vi.fn().mockResolvedValue({
        plugins: [
          {
            pluginId: "io.mender.provider.mcp",
            version: "1.0.0",
            packageVersion: "1.0.0",
            displayName: "MCP",
            description: "Model Context Protocol provider",
            configVersion: 1,
            sourceKind: "builtin",
            capabilities: [],
            enabled: true,
          },
        ],
        issues: [
          {
            installationId: "pinst_00000000000000000000000000000091",
            code: "descriptor_backfill_failed",
            message: "database is read-only",
          },
        ],
      }),
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    expect(
      screen.getByText(
        "Some Provider plugins need attention. Review Plugin Marketplace for installation details.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/descriptor_backfill_failed/)).toBeNull();
    expect(screen.queryByText(/database is read-only/)).toBeNull();
  });

  it("configures an unknown external Provider plugin from its schema", async () => {
    const installationId = "pinst_0123456789abcdef0123456789abcdef";
    const credential = {
      reference: "credential://external/work",
      namespace: "external",
      name: "work",
      displayName: "External work",
      source: "environment" as const,
      environmentVariable: "EXTERNAL_TOKEN",
      createdAt: "2026-09-28T00:00:00Z",
      updatedAt: "2026-09-28T00:00:00Z",
    };
    const plugin = {
      pluginId: "dev.example.echo",
      version: "1.2.3",
      packageVersion: "1.2.3",
      displayName: "External Echo",
      description: "Example external Provider",
      configVersion: 1,
      sourceKind: "external" as const,
      installationId,
      protocolVersion: 1,
      configSchema: {
        type: "object",
        additionalProperties: false,
        required: ["endpoint", "credentialRef"],
        properties: {
          endpoint: {
            type: "string",
            title: "Endpoint",
            format: "uri",
            default: "https://example.test/api",
          },
          credentialRef: {
            type: "string",
            title: "Credential",
            "x-mender-control": "credential",
          },
          mode: {
            type: "string",
            title: "Mode",
            enum: ["fast", "safe"],
            default: "safe",
          },
          enabled: {
            type: "boolean",
            title: "Enabled",
            default: true,
          },
        },
      },
      capabilities: [],
      enabled: true,
    };
    const targetInstallationId = "pinst_1123456789abcdef0123456789abcdef";
    const targetPlugin = {
      ...plugin,
      version: "1.3.0",
      packageVersion: "1.3.0",
      installationId: targetInstallationId,
    };
    const provider = {
      providerId: "prov_external_echo",
      namespace: "external-echo",
      displayName: "External Echo Provider",
      description: "",
      createdAt: "2026-09-28T00:00:00Z",
      updatedAt: "2026-09-28T00:00:00Z",
      pluginId: plugin.pluginId,
      installationId,
      configVersion: 1,
      connectionSummary: "External Echo · production",
      config: {
        endpoint: "https://example.test/api",
        credentialRef: credential.reference,
        mode: "safe",
        enabled: true,
      },
      transport: "unconfigured" as const,
      toolCount: 1,
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValue({ providers: [provider] });
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });
    const updateProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });
    const rebindProvider = vi.fn().mockResolvedValue({
      provider: {
        ...provider,
        installationId: targetInstallationId,
        connectionSummary: "External Echo · upgraded",
      },
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviderPlugins: vi.fn().mockResolvedValue({
        plugins: [
          {
            pluginId: "io.mender.provider.mcp",
            version: "1.0.0",
            packageVersion: "1.0.0",
            displayName: "MCP",
            description: "Model Context Protocol provider",
            configVersion: 1,
            sourceKind: "builtin",
            capabilities: [],
            enabled: true,
          },
          plugin,
          targetPlugin,
        ],
        issues: [],
      }),
      listProviders,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      createProvider,
      updateProvider,
      rebindProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    beginProviderCreate("External Echo · 1.2.3");

    expect(
      screen.queryByRole("button", { name: "Configure authentication" }),
    ).toBeNull();
    expect(screen.getByLabelText<HTMLInputElement>("Endpoint").value).toBe(
      "https://example.test/api",
    );
    expect(screen.getByLabelText<HTMLSelectElement>("Mode").value).toBe("1");
    expect(screen.getByLabelText<HTMLInputElement>("Enabled").checked).toBe(true);

    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "External Echo" },
    });
    expect(screen.getByText("Result: external-echo")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "External Echo Provider" },
    });
    fireEvent.change(screen.getByLabelText("Credential"), {
      target: { value: credential.reference },
    });

    const addButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Add provider",
    });
    expect(addButton.disabled).toBe(false);
    fireEvent.click(addButton);

    await waitFor(() => {
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "external-echo",
        displayName: "External Echo Provider",
        pluginId: "dev.example.echo",
        installationId,
        config: {
          endpoint: "https://example.test/api",
          credentialRef: "credential://external/work",
          mode: "safe",
          enabled: true,
        },
      });
    });
    expect(await screen.findByText("External Echo Provider")).toBeTruthy();
    expect(screen.getByText("External Echo · 1.2.3")).toBeTruthy();
    expect(screen.getByText("External Echo · production")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Rebind External Echo Provider" }),
    );
    expect(await screen.findByText("Rebind Provider installation")).toBeTruthy();
    expect(
      screen.getByLabelText<HTMLSelectElement>(
        "Target Provider plugin installation",
      ).value,
    ).toBe(targetInstallationId);
    expect(screen.getByLabelText<HTMLInputElement>("Endpoint").value).toBe(
      "https://example.test/api",
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Rebind installation" }),
    );
    await waitFor(() => {
      expect(rebindProvider).toHaveBeenCalledWith("prov_external_echo", {
        installationId: targetInstallationId,
        config: {
          endpoint: "https://example.test/api",
          credentialRef: "credential://external/work",
          mode: "safe",
          enabled: true,
        },
      });
    });

    fireEvent.click(
      screen.getByRole("button", { name: "View External Echo Provider" }),
    );
    const details = within(screen.getByRole("dialog"));
    expect(details.getByText(installationId)).toBeTruthy();
    expect(details.getByText("1.2.3")).toBeTruthy();
    expect(details.getByText(/credential:\/\/external\/work/)).toBeTruthy();

    fireEvent.click(
      details.getAllByRole("button", { name: "Close" })[0]!,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Edit External Echo Provider" }),
    );
    expect(
      screen.getByLabelText<HTMLInputElement>("Endpoint").value,
    ).toBe("https://example.test/api");
    fireEvent.change(screen.getByLabelText("Endpoint"), {
      target: { value: "https://example.test/v2" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Save changes" }),
    );

    await waitFor(() => {
      expect(updateProvider).toHaveBeenCalledWith(
        "prov_external_echo",
        {
          displayName: "External Echo Provider",
          config: {
            endpoint: "https://example.test/v2",
            credentialRef: "credential://external/work",
            mode: "safe",
            enabled: true,
          },
        },
      );
    });
  });

  it("fails closed on unsupported external Provider JSON Schema features", async () => {
    const installationId = "pinst_fedcba9876543210fedcba9876543210";
    renderApp("/providers", {
      listProviderPlugins: vi.fn().mockResolvedValue({
        plugins: [
          {
            pluginId: "io.mender.provider.mcp",
            version: "1.0.0",
            packageVersion: "1.0.0",
            displayName: "MCP",
            description: "Model Context Protocol provider",
            configVersion: 1,
            sourceKind: "builtin",
            capabilities: [],
            enabled: true,
          },
          {
            pluginId: "io.mender.provider.mcp",
            version: "1.0.0",
            packageVersion: "1.0.0",
            displayName: "Unsupported External",
            description: "Uses unsupported schema composition",
            configVersion: 1,
            sourceKind: "external",
            installationId,
            protocolVersion: 1,
            configSchema: {
              type: "object",
              oneOf: [
                { properties: { mode: { const: "a" } } },
                { properties: { mode: { const: "b" } } },
              ],
            },
            capabilities: [],
            enabled: true,
          },
        ],
        issues: [],
      }),
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    beginProviderCreate("Unsupported External · 1.0.0");

    expect(screen.queryByText("Remote endpoint")).toBeNull();
    expect(screen.queryByText("Local command")).toBeNull();
    expect(
      screen.getAllByText(/unsupported JSON Schema keyword "oneOf"/).length,
    ).toBeGreaterThanOrEqual(1);
    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "unsupported" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Unsupported" },
    });
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Add provider" })
        .disabled,
    ).toBe(true);
  });

  it("manages Provider plugin installations from the Marketplace", async () => {
    const installation = {
      installationId: "pinst_0123456789abcdef0123456789abcdef",
      pluginId: "dev.example.echo",
      packageVersion: "1.2.3",
      sourceKind: "local" as const,
      digest: "sha256:" + "a".repeat(64),
      status: "enabled" as const,
      installedAt: "2026-09-28T00:00:00Z",
    };
    const disabled = { ...installation, status: "disabled" as const };
    const echo=(status:"enabled"|"disabled")=>({
      key:installation.pluginId+"@"+installation.installationId,
      pluginId:installation.pluginId,extensionId:"main",kind:"provider.tools" as const,
      displayName:"Echo",description:"External Echo",packageVersion:installation.packageVersion,
      sourceKind:"local" as const,status,enabled:status==="enabled",
      capabilities:[],usageCount:0,installationId:installation.installationId,
      digest:installation.digest,installedAt:installation.installedAt,
      configVersion:1,protocolVersion:1,
    });
    const catalogue=vi.fn()
      .mockResolvedValueOnce({plugins:[echo("enabled")]})
      .mockResolvedValueOnce({plugins:[echo("enabled")]})
      .mockResolvedValueOnce({plugins:[echo("disabled")]})
      .mockResolvedValueOnce({plugins:[{
        key:"io.mender.provider.mcp@builtin",pluginId:"io.mender.provider.mcp",
        extensionId:"main",kind:"provider.tools",displayName:"MCP",
        description:"Model Context Protocol Provider",packageVersion:"1.0.0",
        sourceKind:"builtin",status:"enabled",enabled:true,
        capabilities:[],usageCount:0,
      }]});
    const disablePluginInstallation=vi.fn().mockResolvedValue({installation:disabled});
    const deletePluginInstallation=vi.fn().mockResolvedValue(undefined);

    renderApp("/plugins", {
      listPluginCatalogue:catalogue,
      disablePluginInstallation,
      deletePluginInstallation,
    });
    expect(await screen.findByText("Plugin catalog")).toBeTruthy();
    expect(screen.getByText("Echo")).toBeTruthy();
    expect(screen.queryByText("dev.example.echo")).toBeNull();
    expect(screen.queryByText(installation.installationId)).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: "Install plugin",
      }),
    ).toBeNull();

    fireEvent.click(screen.getByRole("link", { name: "View Echo" }));
    expect(await screen.findByRole("button", { name: "Technical details" })).toBeTruthy();
    expect(screen.queryByText("dev.example.echo")).toBeNull();
    expect(screen.queryByText(installation.installationId)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Technical details" }));
    expect(screen.getByText("dev.example.echo")).toBeTruthy();
    expect(screen.getByText(installation.installationId)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Capabilities" }));
    expect(screen.queryByText("dev.example.echo")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Overview" }));
    expect(screen.getByRole("button", { name: "Add provider" })).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", {
        name: "Disable Echo",
      }),
    );
    await waitFor(() => {
      expect(disablePluginInstallation).toHaveBeenCalledWith(
        installation.installationId,
      );
    });
    expect(await screen.findByText("Disabled")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", {name:"Remove Echo"}),
    );
    const removal=screen.getByRole("dialog",{name:"Confirm plugin removal"});
    expect(deletePluginInstallation).not.toHaveBeenCalled();
    fireEvent.click(within(removal).getByRole("button",{name:"Remove"}));
    await waitFor(() => {
      expect(deletePluginInstallation).toHaveBeenCalledWith(
        installation.installationId,
      );
    });
    await waitFor(() => {
      expect(screen.queryByText("Echo")).toBeNull();
    });
    // The foundation driver stays available in the advanced view.
    fireEvent.click(screen.getByRole("button",{name:"Show foundation drivers"}));
    expect(await screen.findByText("MCP")).toBeTruthy();
  });

  it("uses cursor navigation for the Marketplace card grid", async () => {
    const plugins = Array.from({ length: 13 }, (_, index) => ({
      pluginId: `dev.example.plugin-${String(index + 1).padStart(2, "0")}`,
      version: "1.0.0",
      packageVersion: "1.0.0",
      displayName: `Plugin ${String(index + 1).padStart(2, "0")}`,
      description: "Test Provider plugin",
      configVersion: 1,
      sourceKind: "builtin" as const,
      capabilities: [],
      enabled: true,
    }));

    renderApp("/plugins", {
      listPluginCatalogue:vi.fn().mockResolvedValue({plugins:plugins.map(plugin=>({
        key:plugin.pluginId+"@builtin",pluginId:plugin.pluginId,extensionId:"main",
        kind:"provider.tools",displayName:plugin.displayName,description:plugin.description,
        packageVersion:plugin.packageVersion,sourceKind:"builtin",status:"enabled",
        enabled:true,capabilities:[],usageCount:0,configVersion:1,protocolVersion:1,
      }))}),
    });

    expect(
      await screen.findByRole("link", { name: "View Plugin 01" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "View Plugin 12" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: "View Plugin 13" }),
    ).toBeNull();
    expect(screen.queryByText(/Page \d+ of/)).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: "Plugin Marketplace next page",
      }),
    );

    expect(
      await screen.findByRole("link", { name: "View Plugin 13" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: "View Plugin 01" }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: "Plugin Marketplace previous page",
      }),
    );
    expect(
      await screen.findByRole("link", { name: "View Plugin 01" }),
    ).toBeTruthy();
  });

  it("connects reviewed AnySearch without endpoint editing, scripts or implicit Profile grants",async()=>{
    const credential={
      reference:"credential://anysearch/key",namespace:"anysearch",name:"key",
      displayName:"AnySearch Key",source:"native" as const,environmentVariable:"",
      createdAt:"2026-10-12T00:00:00Z",updatedAt:"2026-10-12T00:00:00Z",
    };
    const createProvider=vi.fn().mockResolvedValue({provider:{providerId:"prov_connected"},sync:{created:0,updated:0,removed:0,total:0}});
    const grantAgentProfile=vi.fn();
    renderApp("/plugins",{
      listIntegrationOfferings:vi.fn().mockResolvedValue({integrations:[anySearchOffering]}),
      listProviders:vi.fn().mockResolvedValue({providers:[]}),
      listCredentials:vi.fn().mockResolvedValue({credentials:[credential]}),
      createProvider,grantAgentProfile,
    });
    fireEvent.click(await screen.findByRole("link",{name:"View AnySearch"}));
    expect(await screen.findByRole("button",{name:"Connect"})).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Connect"}));
    const dialog=await screen.findByRole("dialog",{name:"Connect AnySearch"});
    expect(within(dialog).getByText("https://api.anysearch.com/mcp")).toBeTruthy();
    expect(within(dialog).queryByRole("textbox",{name:"Endpoint"})).toBeNull();
    expect(within(dialog).queryByRole("textbox",{name:"Local command"})).toBeNull();
    fireEvent.change(within(dialog).getByRole("combobox",{name:"AnySearch API Credential"}),{
      target:{value:credential.reference},
    });
    fireEvent.click(within(dialog).getByRole("button",{name:"Connect"}));
    await waitFor(()=>expect(createProvider).toHaveBeenCalledWith({
      namespace:"anysearch",displayName:"AnySearch",pluginId:"io.mender.provider.mcp",
      config:{transport:{
        kind:"streamable-http",endpoint:"https://api.anysearch.com/mcp",
        headers:{"X-Anysearch-Client":"mcp/1.0.0"},
        credentialHeaders:[{name:"Authorization",scheme:"Bearer",credentialRef:credential.reference}],
      }},
    }));
    expect(grantAgentProfile).not.toHaveBeenCalled();
  });

  it("opens the existing Provider create flow from a Marketplace detail", async () => {
    renderApp("/plugins", {
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
    });
    expect(await screen.findByRole("button",{name:"Show foundation drivers"})).toBeTruthy();
    expect(screen.queryByRole("link",{name:"View MCP"})).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:"Show foundation drivers"}));

    fireEvent.click(
      await screen.findByRole("link", { name: "View MCP" }),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Add provider" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Add MCP provider",
    });
    expect(within(dialog).getByText("Remote endpoint")).toBeTruthy();
    expect(
      within(dialog).queryByRole("button", {
        name: "Add Model Tools provider",
      }),
    ).toBeNull();
    expect(
      within(dialog).queryByRole("button", { name: "Back to plugins" }),
    ).toBeNull();
  });

  it("preselects the exact external installation from a Provider create link", async () => {
    const installationId = "pinst_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    const plugin = {
      pluginId: "dev.example.echo",
      version: "1.2.3",
      packageVersion: "1.2.3",
      displayName: "External Echo",
      description: "Example external Provider",
      configVersion: 1,
      sourceKind: "external" as const,
      installationId,
      protocolVersion: 1,
      configSchema: {
        type: "object",
        additionalProperties: false,
        properties: {},
      },
      capabilities: [],
      enabled: true,
    };

    renderApp(
      `/providers?create=1&pluginId=dev.example.echo&installationId=${installationId}`,
      {
        listProviderPlugins: vi.fn().mockResolvedValue({
          plugins: [plugin],
          issues: [],
        }),
        listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      },
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Add External Echo · 1.2.3 provider",
    });
    expect(within(dialog).getByText(installationId)).toBeTruthy();
    expect(
      within(dialog).queryByRole("button", { name: "Back to plugins" }),
    ).toBeNull();
  });

  it("shows Marketplace package install only when a host capability is supplied", async () => {
    const installed = {
      installationId: "pinst_fedcba9876543210fedcba9876543210",
      pluginId: "dev.example.local",
      packageVersion: "1.0.0",
      sourceKind: "local" as const,
      digest: "sha256:" + "b".repeat(64),
      status: "enabled" as const,
      installedAt: "2026-09-28T00:00:00Z",
    };
    const installPlugin = vi.fn().mockResolvedValue(installed);
    const installedEntry={
      key:installed.pluginId+"@"+installed.installationId,
      pluginId:installed.pluginId,extensionId:"main",kind:"provider.tools" as const,
      displayName:"Local",description:"Example local plugin",packageVersion:installed.packageVersion,
      sourceKind:"local" as const,status:"enabled" as const,enabled:true,
      capabilities:[],usageCount:0,installationId:installed.installationId,
      digest:installed.digest,installedAt:installed.installedAt,
      configVersion:1,protocolVersion:1,
    };
    const listPluginCatalogue=vi.fn()
      .mockResolvedValueOnce({plugins:[]})
      .mockResolvedValueOnce({plugins:[installedEntry]});
    renderApp("/plugins",{listPluginCatalogue},{installPlugin});

    expect(
      await screen.findByRole("button", {
        name: "Install plugin",
      }),
    ).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Install plugin" }),
    );

    await waitFor(() => {
      expect(installPlugin).toHaveBeenCalledTimes(1);
    });
    expect(await screen.findByText("Local")).toBeTruthy();
  });

  it("keeps the selected built-in Sandbox backend fixed without another chooser",async()=>{
    const pluginId="io.mender.sandbox.local-workspace";
    renderApp("/sandboxes/new?create=1&pluginId="+pluginId,{
      getRuntimeGovernance:vi.fn().mockResolvedValue({backend:{restrictedHostAvailable:true}}),
      listPluginCatalogue:vi.fn().mockResolvedValue({plugins:[{
        key:pluginId+"@builtin",pluginId,extensionId:"main",
        kind:"sandbox.backend",displayName:"Local workspace",description:"Trusted builtin",
        packageVersion:"1.0.0",sourceKind:"builtin",status:"enabled",enabled:true,
        capabilities:["file.read"],usageCount:0,
      }]}),
    });
    expect(await screen.findByLabelText("Sandbox name")).toBeTruthy();
    expect(screen.queryByRole("region",{name:"Choose backend"})).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:"Back"}));
    expect(await screen.findByRole("button",{name:"Capabilities"})).toBeTruthy();
    expect(screen.getByText("Local workspace")).toBeTruthy();
  });

  it("requires an in-app Tunnel deletion confirmation, never native browser confirm",async()=>{
    const item={
      tunnelId:"tun_0123456789abcdef0123456789abcdef",
      displayName:"Fixture Tunnel",provider:"external" as const,
      externalTunnelId:"fixture",runtimeCredentialRef:"credential://example/key",
    };
    const deleteTunnel=vi.fn().mockResolvedValue(undefined);
    const listTunnels=vi.fn()
      .mockResolvedValueOnce({tunnels:[item]})
      .mockResolvedValueOnce({tunnels:[]});
    const confirm=vi.spyOn(window,"confirm").mockImplementation(()=>{
      throw new Error("native confirm must not be called");
    });
    try {
      renderApp("/tunnels",{listTunnels,deleteTunnel});
      expect(await screen.findByText("Fixture Tunnel")).toBeTruthy();
      fireEvent.click(screen.getByRole("button",{name:"Delete Fixture Tunnel"}));
      const dialog=screen.getByRole("dialog",{name:"Confirm Tunnel removal"});
      expect(deleteTunnel).not.toHaveBeenCalled();
      fireEvent.click(within(dialog).getByRole("button",{name:"Cancel"}));
      expect(deleteTunnel).not.toHaveBeenCalled();
      fireEvent.click(screen.getByRole("button",{name:"Delete Fixture Tunnel"}));
      fireEvent.click(within(screen.getByRole("dialog",{name:"Confirm Tunnel removal"}))
        .getByRole("button",{name:"Delete"}));
      await waitFor(()=>expect(deleteTunnel).toHaveBeenCalledWith(item.tunnelId));
    }finally{
      confirm.mockRestore();
    }
  });

  it("automatically refreshes the dormant Docker card when the host connects",async()=>{
    const docker=(ready:boolean)=>({
      key:"io.mender.sandbox.docker@builtin",
      pluginId:"io.mender.sandbox.docker",extensionId:"main",kind:"sandbox.backend" as const,
      displayName:"Docker Engine",description:"Trusted Docker backend",
      packageVersion:"1.0.0",sourceKind:"builtin" as const,
      status:ready?"enabled" as const:"unavailable" as const,enabled:ready,
      capabilities:["file.read","file.search","file.edit","process.shell"],usageCount:0,
      ...(ready?{}:{issue:"Docker Engine not detected"}),
    });
    const listPluginCatalogue=vi.fn()
      .mockResolvedValueOnce({plugins:[docker(false)]})
      .mockResolvedValue({plugins:[docker(true)]});
    const polling=captureDockerPolling();
    try {
      renderApp("/plugins/io.mender.sandbox.docker%40builtin",{listPluginCatalogue});
      expect(await screen.findByText(/Mender Desktop automatically discovers local Docker engines/)).toBeTruthy();
      expect(screen.queryByRole("button",{name:"Create sandbox"})).toBeNull();
      expect(screen.queryByRole("button",{name:"Connect Docker Desktop"})).toBeNull();
      await polling.tick();
      expect(screen.getByRole("button",{name:"Create sandbox"})).toBeTruthy();
      expect(listPluginCatalogue).toHaveBeenCalledTimes(2);
    } finally {
      polling.restore();
    }
  });

  it("updates a connected Docker card on outage and same-Engine recovery",async()=>{
    const docker=(status:"enabled"|"unavailable")=>({
      key:"io.mender.sandbox.docker@builtin",
      pluginId:"io.mender.sandbox.docker",extensionId:"main",kind:"sandbox.backend" as const,
      displayName:"Docker Engine",description:"Trusted Docker backend",
      packageVersion:"1.0.0",sourceKind:"builtin" as const,
      status,enabled:status==="enabled",capabilities:["file.read"],usageCount:0,
      ...(status==="unavailable"?{issue:"Docker Engine disconnected or insecure; retrying"}:{}),
    });
    const listPluginCatalogue=vi.fn()
      .mockResolvedValueOnce({plugins:[docker("enabled")]})
      .mockResolvedValueOnce({plugins:[docker("unavailable")]})
      .mockResolvedValue({plugins:[docker("enabled")]});
    const polling=captureDockerPolling();
    try {
      renderApp("/plugins/io.mender.sandbox.docker%40builtin",{listPluginCatalogue});
      expect(await screen.findByRole("button",{name:"Create sandbox"})).toBeTruthy();
      await polling.tick();
      expect(screen.queryByRole("button",{name:"Create sandbox"})).toBeNull();
      expect(screen.getByText(/Docker Engine disconnected or insecure; retrying/)).toBeTruthy();
      await polling.tick();
      expect(screen.getByRole("button",{name:"Create sandbox"})).toBeTruthy();
      expect(listPluginCatalogue).toHaveBeenCalledTimes(3);
    } finally {
      polling.restore();
    }
  });

  it("keeps unverified Docker unavailable without a privileged Web action",async()=>{
    const pluginId="io.mender.sandbox.docker";
    const listPluginCatalogue=vi.fn().mockResolvedValue({plugins:[{
      key:pluginId+"@builtin",pluginId,extensionId:"main",kind:"sandbox.backend",
      displayName:"Docker Engine",description:"Docker",
      packageVersion:"1.0.0",sourceKind:"builtin",status:"unavailable",enabled:false,
      issue:"Docker Engine not detected",capabilities:["file.read"],usageCount:0,
    }]});
    const polling=captureDockerPolling();
    try {
      renderApp("/plugins/"+pluginId+"%40builtin",{listPluginCatalogue});
      expect(await screen.findByText("Docker Engine")).toBeTruthy();
      expect(screen.getAllByText("Unavailable").length).toBeGreaterThan(0);
      expect(screen.queryByRole("button",{name:"Connect Docker Desktop"})).toBeNull();
      expect(screen.queryByRole("button",{name:"Create sandbox"})).toBeNull();
      await polling.tick();
      expect(listPluginCatalogue).toHaveBeenCalledTimes(2);
      expect(screen.queryByRole("button",{name:"Create sandbox"})).toBeNull();
    } finally {
      polling.restore();
    }
  });

  it("shows all four Sandbox backends with truthful Marketplace statuses and no E2B provisioning",async()=>{
    const builtin=(pluginId:string,displayName:string,status:"enabled"|"unavailable"="enabled")=>({
      key:pluginId+"@builtin",pluginId,extensionId:"main",kind:"sandbox.backend" as const,
      displayName,description:displayName+" Sandbox backend",
      packageVersion:"1.0.0",sourceKind:"builtin" as const,
      status,enabled:status==="enabled",usageCount:0,capabilities:["file.read"],
      ...(status==="unavailable"?{issue:"Experimental Daytona disabled; local operator must set MENDER_DAYTONA_EXPERIMENTAL=enabled before Runtime startup"}:{}),
    });
    const listPluginCatalogue=vi.fn().mockResolvedValue({plugins:[
      builtin("io.mender.sandbox.local-workspace","Local Workspace"),
      builtin("io.mender.sandbox.docker","Docker Engine"),
      builtin("io.mender.sandbox.daytona","Daytona (experimental)","unavailable"),
      {
        key:"io.mender.sandbox.e2b@planned",pluginId:"io.mender.sandbox.e2b",
        extensionId:"main",kind:"sandbox.backend",displayName:"E2B",
        description:"Planned Sandbox backend; not implemented or available for provisioning",
        packageVersion:"",sourceKind:"planned",status:"planned",
        enabled:false,usageCount:0,capabilities:[],
      },
    ]});
    renderApp("/plugins",{listPluginCatalogue});
    for(const displayName of ["Local Workspace","Docker Engine","Daytona (experimental)","E2B"]){
      expect(await screen.findByRole("link",{name:"View "+displayName})).toBeTruthy();
    }
    fireEvent.click(screen.getByRole("link",{name:"View E2B"}));
    expect(await screen.findByText("Planned Sandbox backend; not implemented or available for provisioning")).toBeTruthy();
    expect(screen.getAllByText("Planned").length).toBeGreaterThan(0);
    expect(screen.getByText("Roadmap")).toBeTruthy();
    expect(screen.getByText("Not released")).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Create sandbox"})).toBeNull();
    expect(screen.queryByRole("button",{name:"Enable E2B"})).toBeNull();
    expect(screen.queryByRole("button",{name:"Remove E2B"})).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:"Back"}));
    fireEvent.click(await screen.findByRole("link",{name:"View Daytona (experimental)"}));
    expect(await screen.findByText(/MENDER_DAYTONA_EXPERIMENTAL=enabled/)).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Create sandbox"})).toBeNull();
  });

  it("opens only the selected built-in Sandbox form from the Marketplace",async()=>{
    const local={
      key:"io.mender.sandbox.local-workspace@builtin",
      pluginId:"io.mender.sandbox.local-workspace",extensionId:"main",kind:"sandbox.backend",
      displayName:"Local Workspace",description:"Host workspace",
      packageVersion:"1.0.0",sourceKind:"builtin",status:"enabled",enabled:true,
      usageCount:0,capabilities:["file.read","file.search","file.edit","process.shell"],
    };
    const daytona={...local,
      key:"io.mender.sandbox.daytona@builtin",pluginId:"io.mender.sandbox.daytona",
      displayName:"Daytona (experimental)",description:"Credential-gated experimental",
    };
    const listPluginCatalogue=vi.fn().mockResolvedValue({plugins:[local,daytona]});
    const client={
      listPluginCatalogue,
      getRuntimeGovernance:vi.fn().mockResolvedValue({backend:{restrictedHostAvailable:false}}),
      getSandboxBackends:vi.fn().mockResolvedValue({dockerAvailable:false,dockerStatus:"not configured",daytonaConfigured:true,daytonaStatus:"Experimental"}),
      listCredentials:vi.fn().mockResolvedValue({credentials:[]}),
    };
    renderApp("/plugins/io.mender.sandbox.local-workspace%40builtin",client);
    fireEvent.click(await screen.findByRole("button",{name:"Create sandbox"}));
    expect(await screen.findByRole("textbox",{name:"Sandbox name"})).toBeTruthy();
    expect(screen.getByText("Absolute local directory")).toBeTruthy();
    expect(screen.queryByLabelText("Mender Credential Reference")).toBeNull();
    expect(screen.queryByLabelText("Cached Docker image")).toBeNull();
    // Returning to Marketplace must retain the selected built-in identity.
    fireEvent.click(screen.getByRole("button",{name:"Back"}));
    expect(await screen.findByRole("button",{name:"Create sandbox"})).toBeTruthy();
  });

  it("opens the trusted SSH Remote plugin directly in its own credential-gated create flow",async()=>{
    const pluginId="io.mender.sandbox.ssh-remote";
    const plugin={
      key:pluginId+"@builtin",pluginId,extensionId:"main",kind:"sandbox.backend",
      displayName:"SSH Remote",description:"Pinned SSH remote account",
      packageVersion:"1.0.0",sourceKind:"builtin",status:"enabled",enabled:true,
      capabilities:["file.read","file.search","file.edit","process.shell"],usageCount:0,
    };
    const credential={
      reference:"credential://ssh/build",namespace:"ssh",name:"build",
      displayName:"SSH build",source:"native",environmentVariable:"",
      createdAt:"2026-10-11T00:00:00Z",updatedAt:"2026-10-11T00:00:00Z",
    };
    renderApp("/plugins/"+pluginId+"%40builtin",{
      listPluginCatalogue:vi.fn().mockResolvedValue({plugins:[plugin]}),
      getRuntimeGovernance:vi.fn().mockResolvedValue({backend:{restrictedHostAvailable:false}}),
      getSandboxBackends:vi.fn().mockResolvedValue({dockerAvailable:false,dockerStatus:"unavailable",daytonaConfigured:false,daytonaStatus:"disabled"}),
      listCredentials:vi.fn().mockResolvedValue({credentials:[credential]}),
    });
    fireEvent.click(await screen.findByRole("button",{name:"Create sandbox"}));
    expect(await screen.findByRole("textbox",{name:"SSH host"})).toBeTruthy();
    expect(screen.getByRole("combobox",{name:"SSH private key credential"})).toBeTruthy();
    expect(screen.getByText(/SSH Remote is not container or VM isolation/)).toBeTruthy();
    expect(screen.queryByLabelText("Cached Docker image")).toBeNull();
    expect(screen.queryByLabelText("Mender Credential Reference")).toBeNull();
    const review=screen.getByRole<HTMLButtonElement>("button",{name:"Review"});
    expect(review.disabled).toBe(false);
    fireEvent.click(review);
    expect(screen.getByText("Enter a sandbox name (up to 160 bytes).")).toBeTruthy();
    expect(screen.getByText("Choose an existing SSH private key credential.")).toBeTruthy();
    expect(screen.queryByText("Review Sandbox authority")).toBeNull();
  });

  it("opens the experimental Daytona credential workflow only after native opt-in",async()=>{
    const plugin={
      key:"io.mender.sandbox.daytona@builtin",pluginId:"io.mender.sandbox.daytona",
      extensionId:"main",kind:"sandbox.backend",displayName:"Daytona (experimental)",
      description:"Credential-gated experimental",packageVersion:"1.0.0",
      sourceKind:"builtin",status:"enabled",enabled:true,
      usageCount:0,capabilities:["file.read","file.search","file.edit","process.shell"],
    };
    renderApp("/plugins/io.mender.sandbox.daytona%40builtin",{
      listPluginCatalogue:vi.fn().mockResolvedValue({plugins:[plugin]}),
      getRuntimeGovernance:vi.fn().mockResolvedValue({backend:{restrictedHostAvailable:false}}),
      getSandboxBackends:vi.fn().mockResolvedValue({dockerAvailable:false,dockerStatus:"not configured",daytonaConfigured:true,daytonaStatus:"Experimental"}),
      listCredentials:vi.fn().mockResolvedValue({credentials:[]}),
    });
    fireEvent.click(await screen.findByRole("button",{name:"Create sandbox"}));
    expect(await screen.findByRole("combobox",{name:"Mender Credential Reference"})).toBeTruthy();
    expect(screen.getByText(/No credentials available/)).toBeTruthy();
    expect(screen.queryByLabelText("Absolute local directory")).toBeNull();
    expect(screen.queryByLabelText("Cached Docker image")).toBeNull();
    expect(screen.getByRole<HTMLButtonElement>("button",{name:"Next"}).disabled).toBe(true);
  });

  it("opens a selected Sandbox plugin in its own configuration and review flow",async()=>{
    const installationId="pinst_1234567890abcdef1234567890abcdef";
    const pluginId="dev.example.sandbox";
    const created={sandboxId:"sbx_1234567890abcdef1234567890abcdef",
      name:"Fixture",backend:"external" as const};
    const createExternalSandbox=vi.fn().mockResolvedValue(created);
    renderApp("/sandboxes/new/external?create=1&pluginId="+pluginId+"&installationId="+installationId,{
      listPluginCatalogue:vi.fn().mockResolvedValue({plugins:[{
        key:pluginId+"@"+installationId,pluginId,extensionId:"main",
        kind:"sandbox.backend",displayName:"Fixture Sandbox",description:"Mock",
        packageVersion:"1.0.0",sourceKind:"local",status:"enabled",enabled:true,
        installationId,usageCount:0,capabilities:["file.read"],
        configVersion:1,protocolVersion:1,configSchema:{
          type:"object",properties:{message:{type:"string"}},
          additionalProperties:false,
        },
      }]}),
      createExternalSandbox,
      getSandbox:vi.fn().mockResolvedValue({
        ...created,isolation:"remote",state:"ready",localRoot:"",ownerKind:"local_user",ownerRef:"test",
        activeLeases:0,backendAvailable:true,
      }),
    });
    expect(await screen.findByText("Fixture Sandbox")).toBeTruthy();
    expect(screen.queryByText("Docker")).toBeNull();
    fireEvent.change(screen.getByLabelText("Sandbox name"),{target:{value:"Fixture"}});
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(await screen.findByText(installationId)).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Create sandbox"}));
    await waitFor(()=>expect(createExternalSandbox).toHaveBeenCalledWith({
      name:"Fixture",installationId,config:{},
    }));
  });

  it("opens exactly the selected external Tunnel transport form",async()=>{
    const installationId="pinst_abcdef1234567890abcdef1234567890";
    const pluginId="dev.example.tunnel";
    const createExternalTunnel=vi.fn().mockResolvedValue({
      tunnelId:"tun_11111111111111111111111111111111",
      displayName:"Fixture Tunnel",provider:"external",
      externalTunnelId:"fixture",runtimeCredentialRef:"credential://example/key",
    });
    renderApp("/tunnels?create=1&pluginId="+pluginId+"&installationId="+installationId,{
      listPluginCatalogue:vi.fn().mockResolvedValue({plugins:[{
        key:pluginId+"@"+installationId,pluginId,extensionId:"main",kind:"tunnel.transport",
        displayName:"Fixture Tunnel Transport",description:"Only loopback reachability",
        packageVersion:"1.0.0",sourceKind:"local",status:"enabled",enabled:true,
        installationId,usageCount:0,capabilities:["loopback-mcp-proxy"],
      }]}),
      listCredentials:vi.fn().mockResolvedValue({credentials:[{
        reference:"credential://example/key",namespace:"example",name:"key",
        displayName:"Example",source:"environment",environmentVariable:"EXAMPLE_KEY",
        createdAt:"2026-10-01T00:00:00Z",updatedAt:"2026-10-01T00:00:00Z",
      }]}),
      createExternalTunnel,
    });
    const form=await screen.findByRole("dialog",{name:"Create Tunnel"});
    expectAnchoredDialog("Create Tunnel");
    expect(within(form).getByText("Fixture Tunnel Transport")).toBeTruthy();
    expect(within(form).queryByText("OpenAI Secure MCP Tunnel")).toBeNull();
    fireEvent.change(within(form).getByLabelText("Tunnel display name"),{target:{value:"Fixture Tunnel"}});
    fireEvent.change(within(form).getByLabelText("External Tunnel ID"),{target:{value:"fixture"}});
    fireEvent.click(within(form).getByRole("button",{name:"Create tunnel"}));
    await waitFor(()=>expect(createExternalTunnel).toHaveBeenCalledWith({
      displayName:"Fixture Tunnel",installationId,externalTunnelId:"fixture",
      runtimeCredentialRef:"credential://example/key",
    }));
  });

  it("registers a Model Tools Provider with inventory authentication", async () => {
    const credential = {
      reference: "credential://model/work",
      namespace: "model",
      name: "work",
      displayName: "Model work",
      source: "environment" as const,
      environmentVariable: "MODEL_API_KEY",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const provider = {
      providerId: "prov_model_tools",
      namespace: "research",
      displayName: "Research Search",
      description: "",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
      pluginId: "io.mender.provider.model-tools",
      pluginDisplayName: "Model Tools",
      configVersion: 1,
      transport: "unconfigured" as const,
      toolCount: 1,
      config: {
        engine: "anthropic",
        model: "claude-test",
        credentialRef: credential.reference,
        capabilities: ["web_search"],
      },
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      createProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    beginProviderCreate("Model Tools");
    fireEvent.click(screen.getByRole("button", { name: "Anthropic" }));

    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "research" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Research Search" },
    });
    fireEvent.change(screen.getByLabelText("Model Tools model"), {
      target: { value: "claude-test" },
    });
    expect(screen.getByText("Web search")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Configure authentication" }),
    );
    expect(await screen.findByText("Provider authentication")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Model Tools API credential"), {
      target: { value: credential.reference },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Use authentication" }),
    );

    expect(
      (await screen.findByLabelText<HTMLInputElement>("Model Tools model")).value,
    ).toBe("claude-test");
    const addButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Add provider",
    });
    expect(addButton.disabled).toBe(false);
    fireEvent.click(addButton);

    await waitFor(() => {
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "research",
        displayName: "Research Search",
        pluginId: "io.mender.provider.model-tools",
        config: {
          engine: "anthropic",
          model: "claude-test",
          credentialRef: "credential://model/work",
          capabilities: ["web_search"],
        },
      });
    });
    expect(await screen.findByText("Research Search")).toBeTruthy();
  });

  it("registers an OpenAPI Provider with filters and inventory authentication", async () => {
    const credential = {
      reference: "credential://openapi/work",
      namespace: "openapi",
      name: "work",
      displayName: "OpenAPI work",
      source: "environment" as const,
      environmentVariable: "OPENAPI_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const provider = {
      providerId: "prov_openapi",
      namespace: "example-api",
      displayName: "Example API",
      description: "",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
      pluginId: "io.mender.provider.openapi",
      pluginDisplayName: "OpenAPI",
      configVersion: 1,
      transport: "unconfigured" as const,
      toolCount: 1,
      config: {
        source: {
          kind: "url",
          url: "https://example.test/openapi.yaml",
        },
        serverUrl: "https://api.example.test",
        operations: {
          include: ["getUser"],
          exclude: ["updateUser"],
        },
        credentials: {
          BearerAuth: credential.reference,
        },
      },
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      createProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    beginProviderCreate("OpenAPI");

    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "example-api" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Example API" },
    });
    fireEvent.change(screen.getByLabelText("OpenAPI description URL"), {
      target: { value: "https://example.test/openapi.yaml" },
    });
    fireEvent.change(screen.getByLabelText("OpenAPI server override"), {
      target: { value: "https://api.example.test" },
    });

    fireEvent.click(screen.getByText("Operations"));
    fireEvent.change(screen.getByLabelText("OpenAPI include SourceKeys"), {
      target: { value: "getUser" },
    });
    fireEvent.change(screen.getByLabelText("OpenAPI exclude SourceKeys"), {
      target: { value: "updateUser" },
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Configure authentication" }),
    );
    expect(await screen.findByText("Provider authentication")).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Add Security Scheme binding" }),
    );
    fireEvent.change(screen.getByLabelText("OpenAPI security scheme 1"), {
      target: { value: "BearerAuth" },
    });
    expect(
      screen.getByLabelText<HTMLSelectElement>(
        "OpenAPI security credential 1",
      ).value,
    ).toBe(credential.reference);
    fireEvent.click(
      screen.getByRole("button", { name: "Use authentication" }),
    );

    expect(
      await screen.findByLabelText<HTMLInputElement>("OpenAPI description URL"),
    ).toHaveProperty("value", "https://example.test/openapi.yaml");
    const addButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Add provider",
    });
    expect(addButton.disabled).toBe(false);
    fireEvent.click(addButton);

    await waitFor(() => {
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "example-api",
        displayName: "Example API",
        pluginId: "io.mender.provider.openapi",
        config: {
          source: {
            kind: "url",
            url: "https://example.test/openapi.yaml",
          },
          serverUrl: "https://api.example.test",
          operations: {
            include: ["getUser"],
            exclude: ["updateUser"],
          },
          credentials: {
            BearerAuth: "credential://openapi/work",
          },
        },
      });
    });
    expect(await screen.findByText("Example API")).toBeTruthy();
  });

  it("imports common remote MCP JSON with env-backed headers", async () => {
    const provider = {
      providerId: "prov_anysearch",
      namespace: "anysearch",
      displayName: "anysearch",
      description: "",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
      transport: "http" as const,
      toolCount: 4,
      endpoint: "https://api.anysearch.com/mcp",
      headers: {
        Authorization: "Bearer ${ANYSEARCH_API_KEY}",
      },
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 4, updated: 0, removed: 0, total: 4 },
    });

    renderApp("/providers", {
      listProviders,
      createProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    beginProviderCreate();
    fireEvent.click(screen.getByText("Advanced settings"));

    fireEvent.change(screen.getByLabelText("MCP JSON"), {
      target: {
        value: JSON.stringify({
          mcpServers: {
            anysearch: {
              type: "streamable-http",
              url: "https://api.anysearch.com/mcp",
              headers: {
                Authorization: "Bearer ${ANYSEARCH_API_KEY}",
              },
            },
          },
        }),
      },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Import configuration" }),
    );

    expect(
      screen.getByLabelText<HTMLInputElement>("Namespace").value,
    ).toBe("anysearch");
    expect(
      screen.getByLabelText<HTMLInputElement>("Endpoint").value,
    ).toBe("https://api.anysearch.com/mcp");
    expect(
      screen.getByLabelText<HTMLTextAreaElement>("HTTP header templates").value,
    ).toContain("Authorization: Bearer ${ANYSEARCH_API_KEY}");

    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));

    await waitFor(() => {
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "anysearch",
        displayName: "anysearch",
        pluginId: "io.mender.provider.mcp",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://api.anysearch.com/mcp",
            headers: {
              Authorization: "Bearer ${ANYSEARCH_API_KEY}",
            },
          },
        },
      });
    });
  });

  it("registers a stdio Provider from the same Add dialog", async () => {
    const provider = {
      providerId: "prov_2",
      namespace: "local",
      displayName: "Local Tools",
      description: "",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
      transport: "stdio" as const,
      toolCount: 1,
      executable: "/usr/local/bin/local-tools",
      args: ["--stdio"],
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [provider] });
    const createProvider = vi.fn().mockResolvedValue({
      provider,
      sync: { created: 1, updated: 0, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      createProvider,
    });

    expect(await screen.findByText("No providers")).toBeTruthy();
    beginProviderCreate();
    fireEvent.click(screen.getByRole("button", { name: "Local command" }));

    fireEvent.change(screen.getByLabelText("Namespace"), {
      target: { value: "local" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Local Tools" },
    });
    fireEvent.change(screen.getByLabelText("Executable"), {
      target: { value: "/usr/local/bin/local-tools" },
    });
    fireEvent.change(screen.getByLabelText("Arguments"), {
      target: { value: "--stdio" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add provider" }));

    await waitFor(() => {
      expect(createProvider).toHaveBeenCalledWith({
        namespace: "local",
        displayName: "Local Tools",
        pluginId: "io.mender.provider.mcp",
        config: {
          transport: {
            kind: "stdio",
            executable: "/usr/local/bin/local-tools",
            args: ["--stdio"],
          },
        },
      });
    });
    expect(await screen.findByText("Local Tools")).toBeTruthy();
  });

  it("views and edits a Provider without leaving the list page", async () => {
    const original = {
      providerId: "prov_1",
      namespace: "search",
      displayName: "AnySearch",
      description: "Search provider",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
      transport: "http" as const,
      toolCount: 4,
      endpoint: "https://api.anysearch.com/mcp",
    };
    const updated = {
      ...original,
      displayName: "AnySearch Production",
      description: "Production search provider",
      updatedAt: "2026-09-26T01:00:00Z",
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [original] })
      .mockResolvedValueOnce({ providers: [updated] });
    const updateProvider = vi.fn().mockResolvedValue({
      provider: updated,
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      updateProvider,
    });

    expect(await screen.findByText("AnySearch")).toBeTruthy();
    expect(screen.queryByText("prov_1")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "View AnySearch" }));
    expect(await screen.findByText("Provider details")).toBeTruthy();
    expectAnchoredDialog("Provider details");
    expect(screen.getByText("prov_1")).toBeTruthy();
    expect(
      screen.getAllByText("https://api.anysearch.com/mcp").length,
    ).toBeGreaterThanOrEqual(2);

    const closeButtons = screen.getAllByRole("button", { name: "Close" });
    fireEvent.click(closeButtons[0]!);

    fireEvent.click(screen.getByRole("button", { name: "Edit AnySearch" }));
    expect(await screen.findByText("Edit provider")).toBeTruthy();
    expect(screen.getByText("search")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "AnySearch Production" },
    });
    fireEvent.change(screen.getByLabelText("Description"), {
      target: { value: "Production search provider" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(updateProvider).toHaveBeenCalledWith("prov_1", {
        displayName: "AnySearch Production",
        description: "Production search provider",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://api.anysearch.com/mcp",
          },
        },
      });
    });
    expect(await screen.findByText("AnySearch Production")).toBeTruthy();
  });

  it("configures Provider authentication from Credential inventory", async () => {
    const credential = {
      reference: "credential://github/work",
      namespace: "github",
      name: "work",
      displayName: "GitHub work",
      source: "environment" as const,
      environmentVariable: "GITHUB_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const provider = {
      providerId: "prov_1",
      namespace: "search",
      displayName: "Search",
      description: "",
      createdAt: "2026-09-26T00:00:00Z",
      updatedAt: "2026-09-26T00:00:00Z",
      pluginId: "io.mender.provider.mcp",
      pluginDisplayName: "MCP",
      configVersion: 1,
      transport: "http" as const,
      toolCount: 1,
      endpoint: "https://example.test/mcp",
    };
    const updated = {
      ...provider,
      credentialHeaders: [
        {
          name: "Authorization",
          scheme: "Bearer",
          credentialRef: credential.reference,
        },
      ],
    };
    const listProviders = vi
      .fn()
      .mockResolvedValueOnce({ providers: [provider] })
      .mockResolvedValue({ providers: [updated] });
    const updateProvider = vi.fn().mockResolvedValue({
      provider: updated,
      sync: { created: 0, updated: 1, removed: 0, total: 1 },
    });

    renderApp("/providers", {
      listProviders,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      updateProvider,
    });

    expect(await screen.findByText("Search")).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Authentication Search" }),
    );

    expect(await screen.findByText("Provider authentication")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByLabelText("Credential reference")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Add credential header" }),
    );
    expect(
      screen.getByLabelText<HTMLSelectElement>(
        "Credential header credential 1",
      ).value,
    ).toBe(credential.reference);

    fireEvent.click(
      screen.getByRole("button", { name: "Save authentication" }),
    );

    await waitFor(() => {
      expect(updateProvider).toHaveBeenCalledWith("prov_1", {
        displayName: "Search",
        config: {
          transport: {
            kind: "streamable-http",
            endpoint: "https://example.test/mcp",
            credentialHeaders: [
              {
                name: "Authorization",
                scheme: "Bearer",
                credentialRef: "credential://github/work",
              },
            ],
          },
        },
      });
    });
  });

  it("filters Catalog browse and search by Provider", async () => {
    const getTool = vi.fn().mockResolvedValue({
      toolId: "tool_1",
      providerId: "prov_1",
      sourceKey: "issues.create",
      name: "create_issue",
      title: "Create issue",
      description: "Create a repository issue",
      inputSchema: { type: "object" },
      outputSchema: { type: "object" },
      tags: ["mcp"],
    });

    const browseTools = vi.fn().mockResolvedValue({
      tools: [
        {
          rank: 1,
          toolId: "tool_browse",
          providerId: "prov_1",
          name: "web_search",
          title: "Web search",
          description: "Browse synchronized Tool",
        },
      ],
    });
    const searchTools = vi.fn().mockResolvedValue({
      tools: [
        {
          rank: 1,
          toolId: "tool_1",
          providerId: "prov_1",
          name: "create_issue",
          title: "Create issue",
          description: "Create a repository issue",
        },
      ],
    });

    renderApp("/catalog", {
      listProviders: vi.fn().mockResolvedValue({
        providers: [
          {
            providerId: "prov_1",
            namespace: "demo",
            displayName: "Demo",
            description: "",
            createdAt: "2026-09-26T00:00:00Z",
            updatedAt: "2026-09-26T00:00:00Z",
            transport: "http",
            toolCount: 2,
            endpoint: "https://example.test/mcp",
          },
        ],
      }),
      browseTools,
      searchTools,
      getTool,
    });

    expect(await screen.findByText("Web search")).toBeTruthy();
    expect(getTool).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("Catalog profile filter")).toBeNull();
    expect(screen.getByRole("table").classList.contains("table-fixed")).toBe(
      true,
    );
    expect(screen.getByRole("columnheader", { name: "Rank" }).classList).toContain(
      "hidden",
    );
    expect(
      screen.getByRole("columnheader", { name: "Actions" }),
    ).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Catalog provider filter"), {
      target: { value: "prov_1" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Browse" }));
    await waitFor(() => {
      expect(browseTools).toHaveBeenLastCalledWith({
        limit: 100,
        providerId: "prov_1",
      });
    });

    fireEvent.change(
      screen.getByPlaceholderText("Search intent, e.g. create issue"),
      { target: { value: "create issue" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(searchTools).toHaveBeenCalledWith({
        query: "create issue",
        limit: 30,
        providerId: "prov_1",
      });
    });
    expect(await screen.findByText("Create issue")).toBeTruthy();
    expect(getTool).not.toHaveBeenCalled();
    expect(screen.queryByText("Input schema")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Create issue/ }));

    expect(await screen.findByText("Create issue")).toBeTruthy();
    expectAnchoredDialog("Create issue");
    expect(getTool).toHaveBeenCalledWith("tool_1");
    expect(within(screen.getByRole("dialog")).getByText("Description")).toBeTruthy();
    expect(screen.queryByText("Input schema")).toBeNull();
    expect(screen.queryByLabelText("Invocation arguments")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Read full description" }),
    );
    expect(await screen.findByText("Tool description")).toBeTruthy();
    expect(screen.queryByText("Input schema")).toBeNull();
    expect(screen.queryByLabelText("Invocation arguments")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    fireEvent.click(screen.getByRole("button", { name: "View schemas" }));
    expect(await screen.findByText("Input schema")).toBeTruthy();
    expect(screen.queryByLabelText("Invocation arguments")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    fireEvent.click(screen.getByRole("button", { name: "Invoke tool" }));
    expect(await screen.findByLabelText("Invocation arguments")).toBeTruthy();
    expect(screen.queryByText("Input schema")).toBeNull();
  });

  it("creates an OpenAI Tunnel from Credential inventory without raw secret input", async () => {
    const credential = {
      reference: "credential://openai/tunnel-runtime",
      namespace: "openai",
      name: "tunnel-runtime",
      displayName: "OpenAI Tunnel Runtime",
      source: "environment" as const,
      environmentVariable: "OPENAI_TUNNEL_RUNTIME_KEY",
      createdAt: "2026-10-01T00:00:00Z",
      updatedAt: "2026-10-01T00:00:00Z",
    };
    const tunnel = {
      tunnelId: "tun_00000000000000000000000000000001",
      displayName: "OpenAI Work",
      provider: "openai" as const,
      externalTunnelId: "tunnel_external_work",
      runtimeCredentialRef: credential.reference,
    };
    const listTunnels = vi
      .fn()
      .mockResolvedValueOnce({ tunnels: [] })
      .mockResolvedValue({ tunnels: [tunnel] });
    const createOpenAITunnel = vi.fn().mockResolvedValue(tunnel);

    renderApp("/tunnels", {
      listTunnels,
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      createOpenAITunnel,
    });

    expect(await screen.findByText("No Tunnels")).toBeTruthy();
    expect(
      screen.getByText(
        "Add a Tunnel to make a managed local Profile MCP server reachable from external clients.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "OpenAI Secure MCP Tunnel" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Add tunnel" }));
    const chooser=await screen.findByRole("dialog",{name:"Choose Tunnel transport"});
    fireEvent.click(within(chooser).getByRole("button",{
      name:/OpenAI Secure MCP Tunnel/,
    }));
    expect(await screen.findByRole("dialog",{name:"Create Tunnel"})).toBeTruthy();
    expect(screen.queryByLabelText("Credential reference")).toBeNull();
    expect(screen.queryByLabelText("Tunnel client executable")).toBeNull();

    fireEvent.change(screen.getByLabelText("Tunnel display name"), {
      target: { value: "OpenAI Work" },
    });
    fireEvent.change(screen.getByLabelText("External Tunnel ID"), {
      target: { value: "tunnel_external_work" },
    });
    expect(
      screen.getByLabelText<HTMLSelectElement>("Runtime API-key Credential")
        .value,
    ).toBe(credential.reference);

    fireEvent.click(screen.getByRole("button", { name: "Create tunnel" }));

    await waitFor(() => {
      expect(createOpenAITunnel).toHaveBeenCalledWith({
        displayName: "OpenAI Work",
        externalTunnelId: "tunnel_external_work",
        runtimeCredentialRef: credential.reference,
      });
    });
    expect(await screen.findByText("OpenAI Work")).toBeTruthy();
  });

  it("binds and clears a Profile Tunnel through inventory selection", async () => {
    const profile = {
      profileId: "prof_1",
      slug: "default",
      displayName: "Default",
      mode: "hybrid" as const,
      tools: [],
    };
    const tunnel = {
      tunnelId: "tun_00000000000000000000000000000001",
      displayName: "OpenAI Work",
      provider: "openai" as const,
      externalTunnelId: "tunnel_external_work",
      runtimeCredentialRef: "credential://openai/tunnel-runtime",
    };
    const setProfileTunnel = vi
      .fn()
      .mockImplementation((_slug: string, body: { tunnelId: string }) =>
        Promise.resolve(
          body.tunnelId
            ? { ...profile, tunnelId: body.tunnelId }
            : { ...profile },
        ),
      );

    renderApp("/profiles", {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      listTunnels: vi.fn().mockResolvedValue({ tunnels: [tunnel] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [] }),
      setProfileTunnel,
    });

    expect((await screen.findAllByText("Default")).length).toBeGreaterThan(0);
    expect(screen.getByText("Local only")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Manage Default" }));
    fireEvent.click(screen.getByRole("button", { name: "Overview" }));
    const tunnelSelect =
      await screen.findByLabelText<HTMLSelectElement>("Profile tunnel");
    expect(tunnelSelect.value).toBe("");

    fireEvent.change(tunnelSelect, { target: { value: tunnel.tunnelId } });
    await waitFor(() => {
      expect(setProfileTunnel).toHaveBeenLastCalledWith("default", {
        tunnelId: tunnel.tunnelId,
      });
    });
    expect(
      screen.getByLabelText<HTMLSelectElement>("Profile tunnel").value,
    ).toBe(tunnel.tunnelId);

    fireEvent.change(screen.getByLabelText("Profile tunnel"), {
      target: { value: "" },
    });
    await waitFor(() => {
      expect(setProfileTunnel).toHaveBeenLastCalledWith("default", {
        tunnelId: "",
      });
    });
  });

  it("manages Profile membership through a Tool picker without ToolID input", async () => {
    const profile = {
      profileId: "prof_1",
      slug: "default",
      displayName: "Default",
      mode: "hybrid" as const,
      tools: [
        {
          toolId: "tool_1",
          direct: true,
          alias: "search_prod",
          titleOverride: "",
          descriptionOverride: "",
          policyConstraint: "inherit" as const,
          concurrencyConstraint: "serial" as const,
          credentialRefs: [],
        },
      ],
    };
    const credential = {
      reference: "credential://github/work",
      namespace: "github",
      name: "work",
      displayName: "GitHub work",
      source: "environment" as const,
      environmentVariable: "GITHUB_TOKEN",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const replaceProfileTools = vi.fn().mockImplementation(
      (_slug: string, body: { tools: Array<Record<string, unknown>> }) =>
        Promise.resolve({
          ...profile,
          tools: body.tools.map((tool) => ({
            direct: false,
            alias: "",
            titleOverride: "",
            descriptionOverride: "",
            policyConstraint: "inherit" as const,
            concurrencyConstraint: "inherit" as const,
            credentialRefs: [],
            ...tool,
          })),
        }),
    );

    renderApp("/profiles", {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
      listCredentials: vi.fn().mockResolvedValue({ credentials: [credential] }),
      listProviders: vi.fn().mockResolvedValue({
        providers: [
          {
            providerId: "prov_1",
            namespace: "search",
            displayName: "AnySearch",
            description: "",
            createdAt: "2026-09-26T00:00:00Z",
            updatedAt: "2026-09-26T00:00:00Z",
            transport: "http",
            toolCount: 2,
            endpoint: "https://api.anysearch.com/mcp",
          },
        ],
      }),
      browseTools: vi.fn().mockResolvedValue({
        tools: [
          {
            rank: 1,
            toolId: "tool_1",
            providerId: "prov_1",
            name: "search",
            title: "Search",
            description: "Search the web",
          },
          {
            rank: 2,
            toolId: "tool_2",
            providerId: "prov_1",
            name: "extract",
            title: "Extract",
            description: "Extract content",
          },
        ],
      }),
      replaceProfileTools,
    });

    expect((await screen.findAllByText("Default")).length).toBeGreaterThan(0);
    expect(screen.queryByText("tool_1")).toBeNull();
    expect(screen.queryByPlaceholderText("tool_...")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Manage Default" }));
    expect(await screen.findByText("Search")).toBeTruthy();
    expect(screen.queryByText("tool_1")).toBeNull();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Choose tools" }));
    expect(await screen.findByText("Extract")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.getByText("Extract content").classList.contains("line-clamp-2")).toBe(
      true,
    );
    expect(
      screen.getByRole<HTMLInputElement>("checkbox", { name: "Select Extract" })
        .disabled,
    ).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("button", { name: "Choose tools" })).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Choose tools" }));
    expect(await screen.findByText("Extract")).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: "Select Extract" }));
    fireEvent.click(screen.getByRole("button", { name: "Save selection" }));

    await waitFor(() => {
      expect(replaceProfileTools).toHaveBeenCalledWith("default", {
        tools: [
          {
            toolId: "tool_1",
            direct: true,
            alias: "search_prod",
            titleOverride: "",
            descriptionOverride: "",
            policyConstraint: "inherit",
            concurrencyConstraint: "serial",
            credentialRefs: [],
          },
          { toolId: "tool_2", direct: false },
        ],
      });
    });
    expect(await screen.findByText("Extract")).toBeTruthy();
    expect(screen.getByText("Alias: search_prod")).toBeTruthy();
    expect(screen.getByText("Serial")).toBeTruthy();
    expect(screen.queryByPlaceholderText("tool_...")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Presentation for Search" }),
    );
    expect(await screen.findByText("Tool presentation")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Choose tools" })).toBeNull();

    fireEvent.change(screen.getByLabelText("Tool binding alias"), {
      target: { value: "Search Release" },
    });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save presentation" }).disabled).toBe(true);
    expect(screen.getByText(/Tool aliases preserve case/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Tool binding alias"), {
      target: { value: "Search.Release_2" },
    });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save presentation" }).disabled).toBe(false);
    fireEvent.change(screen.getByLabelText("Tool binding title override"), {
      target: { value: "Release Search" },
    });
    const longDescription =
      "Search release documentation with a deliberately long Profile-specific description that remains truncated in the selected Tool row.";
    fireEvent.change(screen.getByLabelText("Tool binding description override"), {
      target: { value: longDescription },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save presentation" }));

    expect(await screen.findByText("Release Search")).toBeTruthy();
    expect(screen.getByText("Alias: Search.Release_2")).toBeTruthy();
    expect(screen.getByText(/Search release documentation/).classList).toContain(
      "truncate",
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Governance for Release Search" }),
    );
    expect(await screen.findByText("Tool governance")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.change(screen.getByLabelText("Binding policy constraint"), {
      target: { value: "deny" },
    });
    fireEvent.change(screen.getByLabelText("Binding concurrency constraint"), {
      target: { value: "serial" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save governance" }));

    expect(await screen.findByText("Denied")).toBeTruthy();
    expect(screen.getByText("Serial")).toBeTruthy();
    expect(screen.queryByText("tool_1")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Credentials for Release Search" }),
    );
    expect(await screen.findByText("Tool credentials")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByLabelText("Credential reference")).toBeNull();
    fireEvent.click(
      screen.getByRole("checkbox", { name: /GitHub work/ }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Save credentials" }));

    await waitFor(() => {
      const lastCall = replaceProfileTools.mock.lastCall as unknown as [
        string,
        {
          tools: Array<{
            toolId: string;
            credentialRefs?: string[];
          }>;
        },
      ];
      expect(lastCall[0]).toBe("default");
      expect(
        lastCall[1].tools.find((tool) => tool.toolId === "tool_1")
          ?.credentialRefs,
      ).toEqual(["credential://github/work"]);
    });
    expect(await screen.findByText("1 credential")).toBeTruthy();
  });

  it("starts, restarts, and stops a managed MCP server", async () => {
    const profile = {
      profileId: "prof_1",
      slug: "default",
      displayName: "Default",
      mode: "hybrid" as const,
      tunnelId: "tun_00000000000000000000000000000001",
      tools: [{ toolId: "tool_1", direct: true }],
    };
    const started = {
      serverId: "srv_1",
      profile: "default",
      transport: "streamable-http" as const,
      address: "127.0.0.1:8765",
      endpoint: "http://127.0.0.1:8765/mcp",
      status: "running" as const,
      startedAt: "2026-09-26T09:00:00Z",
      tunnelId: "tun_00000000000000000000000000000001",
      tunnelDisplayName: "OpenAI Work",
      tunnelProvider: "openai" as const,
      tunnelExternalId: "tunnel_external_work",
      tunnelStatus: "running" as const,
    };
    const restarted = {
      ...started,
      serverId: "srv_2",
      endpoint: "http://127.0.0.1:8766/mcp",
      address: "127.0.0.1:8766",
      startedAt: "2026-09-26T09:01:00Z",
    };

    const startMCPServer = vi.fn().mockResolvedValue(started);
    const restartMCPServer = vi.fn().mockResolvedValue(restarted);
    const stopMCPServer = vi.fn().mockResolvedValue(undefined);

    renderApp("/servers", {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
      listMCPServers: vi.fn().mockResolvedValue({ servers: [] }),
      startMCPServer,
      restartMCPServer,
      stopMCPServer,
    });

    expect(await screen.findByText("No running MCP servers")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Start server" }));
    expect(await screen.findByText("Start MCP server")).toBeTruthy();
    expectAnchoredDialog("Start MCP server");
    expect(
      screen.getByLabelText<HTMLSelectElement>("Server profile").value,
    ).toBe("default");
    expect(
      screen.getByText(/Starting this Profile also starts/),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Start server" }));

    await waitFor(() => {
      expect(startMCPServer).toHaveBeenCalledWith({
        profile: "default",
        address: "127.0.0.1:8765",
      });
    });
    expect(await screen.findByText(started.endpoint)).toBeTruthy();
    expect(screen.getByText("OpenAI Work")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Restart" }));
    await waitFor(() => {
      expect(restartMCPServer).toHaveBeenCalledWith("srv_1");
    });
    expect(await screen.findByText(restarted.endpoint)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Stop" }));
    await waitFor(() => {
      expect(stopMCPServer).toHaveBeenCalledWith("srv_2");
    });
    expect(await screen.findByText("No running MCP servers")).toBeTruthy();
  });

  it("manages Policy rules and Credential inventory without raw identities or secret echo", async () => {
    const policyProfile = {
      profileId: "prof_00000000000000000000000000000001",
      slug: "default",
      displayName: "Default",
      mode: "hybrid" as const,
      tools: [],
    };
    const policyProvider = {
      providerId: "prov_00000000000000000000000000000001",
      namespace: "demo",
      displayName: "Demo Provider",
      description: "",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
      transport: "http" as const,
      toolCount: 1,
      endpoint: "https://example.test/mcp",
    };
    const policyTool = {
      rank: 1,
      toolId: "tool_00000000000000000000000000000001",
      providerId: policyProvider.providerId,
      name: "search",
      title: "Search",
      description: "Search documentation",
    };
    const createdRule = {
      ruleId: "rule_00000000000000000000000000000001",
      displayName: "Approve MCP search",
      enabled: true,
      decision: "require_approval" as const,
      match: {
        actions: ["tool.invoke"] as Array<"tool.invoke">,
        origins: ["mcp"] as Array<"mcp">,
        principalKinds: ["service"] as Array<"service">,
        principalRefs: [],
        scopeKinds: ["profile"] as Array<"profile">,
        profileRefs: [policyProfile.profileId],
        providerIds: [policyProvider.providerId],
        toolIds: [policyTool.toolId],
        factKnowledge: "unknown" as const,
        effectsAny: [],
        resourcesAny: [],
      },
      createdAt: "2026-09-27T01:00:00Z",
      updatedAt: "2026-09-27T01:00:00Z",
    };
    const getPolicy = vi
      .fn()
      .mockResolvedValueOnce({ defaultDecision: "allow", rules: [] })
      .mockResolvedValueOnce({ defaultDecision: "deny", rules: [] })
      .mockResolvedValue({ defaultDecision: "deny", rules: [createdRule] });
    const getPolicyStatus = vi
      .fn()
      .mockResolvedValueOnce({
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "allow",
        ruleCount: 0,
        enabledRuleCount: 0,
      })
      .mockResolvedValueOnce({
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "deny",
        ruleCount: 0,
        enabledRuleCount: 0,
      })
      .mockResolvedValue({
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "deny",
        ruleCount: 1,
        enabledRuleCount: 1,
      });
    const setPolicyDefault = vi.fn().mockResolvedValue({
      defaultDecision: "deny",
      rules: [],
    });
    const createPolicyRule = vi.fn().mockResolvedValue(createdRule);

    const policyClient = renderApp("/policy", {
      getPolicy,
      getPolicyStatus,
      setPolicyDefault,
      createPolicyRule,
      listProfiles: vi.fn().mockResolvedValue({ profiles: [policyProfile] }),
      listProviders: vi.fn().mockResolvedValue({ providers: [policyProvider] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [policyTool] }),
    });

    expect(await screen.findByText("No Policy Rules")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(await screen.findByText("Edit default decision")).toBeTruthy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.change(screen.getByLabelText("Default Policy decision"), {
      target: { value: "deny" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save default" }));
    await waitFor(() => {
      expect(setPolicyDefault).toHaveBeenCalledWith({ decision: "deny" });
    });

    fireEvent.click(screen.getByRole("button", { name: "Add rule" }));
    expect(await screen.findByText("Add Policy rule")).toBeTruthy();
    expectAnchoredDialog("Add Policy rule");
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByLabelText("Rule ID")).toBeNull();
    fireEvent.change(screen.getByLabelText("Policy Rule display name"), {
      target: { value: "Approve MCP search" },
    });
    fireEvent.change(screen.getByLabelText("Policy Rule decision"), {
      target: { value: "require_approval" },
    });
    expect(screen.getByText(/Approval adapter is fail-closed/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(screen.getByText(/Scope and targets/)).toBeTruthy();
    fireEvent.click(screen.getByText("MCP").closest("label")!);
    fireEvent.click(screen.getByText("Profile").closest("label")!);
    fireEvent.click(screen.getByText("Default").closest("label")!);
    fireEvent.click(screen.getByText("Demo Provider").closest("label")!);
    fireEvent.click(screen.getByText("Search").closest("label")!);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(screen.getByText("Governance Facts")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Policy fact knowledge"), {
      target: { value: "unknown" },
    });
    expect(screen.queryByLabelText("Policy effects")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Create rule" }));

    await waitFor(() => {
      expect(createPolicyRule).toHaveBeenCalledTimes(1);
    });
    const createInput = createPolicyRule.mock.calls[0]?.[0] as PolicyRuleInput;
    expect(createInput).toMatchObject({
      displayName: "Approve MCP search",
      decision: "require_approval",
      match: {
        actions: ["tool.invoke"],
        origins: ["mcp"],
        principalKinds: [],
        principalRefs: [],
        scopeKinds: ["profile"],
        profileRefs: [policyProfile.profileId],
        providerIds: [policyProvider.providerId],
        toolIds: [policyTool.toolId],
        factKnowledge: "unknown",
      },
    });
    expect("ruleId" in createInput).toBe(false);
    expect(await screen.findByText("Approve MCP search")).toBeTruthy();
    policyClient.unmount();

    const credential = {
      reference: "credential://github/work",
      namespace: "github",
      name: "work",
      displayName: "GitHub work",
      source: "native" as const,
      environmentVariable: "",
      createdAt: "2026-09-27T00:00:00Z",
      updatedAt: "2026-09-27T00:00:00Z",
    };
    const listCredentials = vi
      .fn()
      .mockResolvedValueOnce({ credentials: [] })
      .mockResolvedValue({ credentials: [credential] });
    const createNativeCredential = vi.fn().mockResolvedValue(credential);
    const rotateCredential = vi.fn().mockResolvedValue({
      ...credential,
      updatedAt: "2026-09-27T01:00:00Z",
    });

    renderApp("/credentials", {
      listCredentials,
      createNativeCredential,
      rotateCredential,
    });

    expect(await screen.findByText("No Credentials")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add credential" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.queryByLabelText("Credential reference")).toBeNull();

    fireEvent.change(screen.getByLabelText("Credential namespace"), {
      target: { value: "github" },
    });
    fireEvent.change(screen.getByLabelText("Credential name"), {
      target: { value: "work" },
    });
    fireEvent.change(screen.getByLabelText("Credential display name"), {
      target: { value: "GitHub work" },
    });
    fireEvent.change(screen.getByLabelText("Credential secret"), {
      target: { value: "TOP-SECRET" },
    });
    expect(screen.getByText("credential://github/work")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    await waitFor(() => {
      expect(createNativeCredential).toHaveBeenCalledWith({
        namespace: "github",
        name: "work",
        displayName: "GitHub work",
        secret: "TOP-SECRET",
      });
    });

    expect(await screen.findByText("GitHub work")).toBeTruthy();
    expect(screen.queryByText("credential://github/work")).toBeNull();
    expect(screen.queryByText("TOP-SECRET")).toBeNull();
    expect(screen.queryByDisplayValue("TOP-SECRET")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Rotate" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.change(screen.getByLabelText("New Credential secret"), {
      target: { value: "NEW-SECRET" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Rotate secret" }));
    await waitFor(() => {
      expect(rotateCredential).toHaveBeenCalledWith("github", "work", {
        secret: "NEW-SECRET",
      });
    });
    expect(screen.queryByText("NEW-SECRET")).toBeNull();
  });

  it("edits Runtime budget and reports restricted-host availability", async () => {
    const status = {
      budget: {
        maxInvocations: 32,
        maxPrincipalInvocations: 8,
        maxProviderInvocations: 8,
        maxToolInvocations: 4,
        maxProviderActivations: 4,
        maxExecutionTimeoutMs: 120000,
        maxArgumentBytes: 1048576,
        maxResultBytes: 1048576,
      },
      isolation: {
        workspaceShell: "host" as const,
        externalProvider: "host" as const,
        tunnelClient: "host" as const,
      },
      backend: { name: "none", restrictedHostAvailable: false },
      usage: { invocations: 1, activations: 0 },
    };
    const updateRuntimeBudget = vi
      .fn<MenderClient["updateRuntimeBudget"]>()
      .mockImplementation((budget) => Promise.resolve({ ...status, budget }));
    const updateRuntimeIsolation = vi
      .fn<MenderClient["updateRuntimeIsolation"]>()
      .mockImplementation((isolation) => Promise.resolve({ ...status, isolation }));

    renderApp("/runtime", {
      getRuntimeGovernance: vi.fn().mockResolvedValue(status),
      updateRuntimeBudget,
      updateRuntimeIsolation,
    });

    expect(await screen.findByRole("group", { name: "Runtime governance" })).toBeTruthy();
    expect(screen.queryByLabelText("Tunnel client")).toBeNull();
    expect(screen.queryByLabelText("Global invocations")).toBeNull();
    expect(screen.queryByText("restricted_host is unavailable on this Runtime. Host mode keeps the local process authority of the Runtime user.")).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Runtime budget" }));
    expect(screen.queryByLabelText("Global invocations")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expectAnchoredDialog("Runtime budget");
    const globalInput = screen.getByLabelText("Global invocations");
    fireEvent.change(globalInput, { target: { value: "12" } });
    fireEvent.click(screen.getByRole("button", { name: "Save budget" }));

    await waitFor(() => {
      expect(updateRuntimeBudget).toHaveBeenCalledWith({
        ...status.budget,
        maxInvocations: 12,
      });
    });

    fireEvent.click(screen.getByRole("button", { name: "Process isolation" }));
    expect(screen.getByText("restricted_host is unavailable on this Runtime. Host mode keeps the local process authority of the Runtime user.")).toBeTruthy();
    expect(screen.queryByLabelText("Tunnel client")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(screen.getByLabelText("Tunnel client")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save isolation" }));
    await waitFor(() => {
      expect(updateRuntimeIsolation).toHaveBeenCalledWith(status.isolation);
    });
  });

  it("renders only minimized Audit metadata", async () => {
    renderApp("/audit", {
      listProviderActivationAudit: vi.fn().mockResolvedValue({ records: [] }),
      listAudit: vi.fn().mockResolvedValue({
        records: [
          {
            invocationId: "inv_1",
            toolId: "tool_1",
            providerId: "prov_1",
            principalKind: "local_user",
            principalRef: "os:501",
            startedAt: "2026-09-26T00:00:00Z",
            finishedAt: "2026-09-26T00:00:01Z",
            policyKind: "allow",
            policyReason: "local.default",
            approval: "not_required",
            status: "completed",
            resultKind: "complete",
            errorClass: "",
            errorSummary: "",
            origin: "management_http",
            scopeKind: "catalog",
            scopeReference: "",
            outcomeKind: "complete",
            rejectionCode: "",
            failureSource: "",
            failureCode: "",
          },
        ],
      }),
    });

    expect(await screen.findByText("Completed")).toBeTruthy();
    expect(screen.getByText("Unknown Tool")).toBeTruthy();
    expect(screen.getByRole("table", { name: "Invocation history" })).toBeTruthy();
    expect(screen.queryByRole("table", { name: "Provider activation history" })).toBeNull();
    expect(screen.queryByText("tool_1")).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Arguments" })).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Result" })).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Secret" })).toBeNull();
  });

  it("shows a bounded Audit failure summary in a detail dialog", async () => {
    renderApp("/audit", {
      listProviderActivationAudit: vi.fn().mockResolvedValue({ records: [] }),
      listAudit: vi.fn().mockResolvedValue({
        records: [
          {
            invocationId: "inv_failed",
            toolId: "tool_1",
            providerId: "prov_1",
            principalKind: "local_user",
            principalRef: "os:501",
            startedAt: "2026-09-26T00:00:00Z",
            finishedAt: "2026-09-26T00:00:01Z",
            policyKind: "allow",
            policyReason: "local.default",
            approval: "not_required",
            status: "failed",
            resultKind: "",
            errorClass: "execution",
            errorSummary: "provider rejected request: missing required query",
            origin: "desktop",
            scopeKind: "profile",
            scopeReference: "prof_1",
            outcomeKind: "failed",
            rejectionCode: "",
            failureSource: "provider",
            failureCode: "provider_error",
          },
        ],
      }),
    });

    expect(await screen.findByText("Failed")).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "View audit record" }),
    );

    const dialog = within(screen.getByRole("dialog", { name: "Invocation details" }));
    expect(dialog.getByRole("button", { name: "Diagnostics" }).getAttribute("aria-pressed")).toBe("true");
    expect(dialog.getByText("provider rejected request: missing required query")).toBeTruthy();
    expect(dialog.getByText("execution")).toBeTruthy();
    expect(dialog.getByText("provider")).toBeTruthy();
    expect(dialog.getByText("provider_error")).toBeTruthy();
    expect(dialog.queryByText("tool_1")).toBeNull();
    fireEvent.click(dialog.getByRole("button", { name: "Overview" }));
    expect(dialog.getByText("tool_1")).toBeTruthy();
    expect(dialog.getByText("desktop")).toBeTruthy();
    expect(dialog.getByText("local_user:os:501")).toBeTruthy();
    fireEvent.click(dialog.getByRole("button", { name: "Governance" }));
    expect(dialog.getByText("profile")).toBeTruthy();
    expect(dialog.getByText("local.default")).toBeTruthy();
    expect(dialog.queryByText("provider_error")).toBeNull();
    expect(screen.queryByText(/TOP-SECRET/)).toBeNull();
  });
});

function beginProviderCreate(pluginLabel = "MCP") {
  fireEvent.click(screen.getByRole("button", { name: "Add provider" }));
  const chooser = screen.getByRole("dialog", {
    name: "Choose Provider plugin",
  });
  fireEvent.click(
    within(chooser).getByRole("button", {
      name: `Add ${pluginLabel} provider`,
    }),
  );
}

function captureDockerPolling(){
  let refresh:(()=>void)|undefined;
  const original=window.setInterval.bind(window);
  const spy=vi.spyOn(window,"setInterval").mockImplementation((handler,delay)=>{
    if(delay===5000 && typeof handler==="function"){
      refresh=handler as ()=>void;
      return -1;
    }
    return original(handler,delay);
  });
  return {
    async tick(){
      if(!refresh)throw new Error("Docker auto-refresh timer missing");
      await act(async()=>{refresh?.();await Promise.resolve();});
    },
    restore(){spy.mockRestore();},
  };
}

const anySearchOffering = {
  offeringId:"io.mender.integration.anysearch",
  displayName:"AnySearch",
  description:"Real-time search through hosted MCP",
  kind:"remote-mcp" as const,
  driverPluginId:"io.mender.provider.mcp" as const,
  transport:"streamable-http" as const,
  endpoint:"https://api.anysearch.com/mcp",
  headers:{"X-Anysearch-Client":"mcp/1.0.0"},
  defaultNamespace:"anysearch",
  authentication:"optional-bearer" as const,
  sourceUrl:"https://github.com/anysearch-ai/anysearch-mcp-server",
};

function renderApp(
  path: string,
  client: Partial<MenderClient>,
  options: {
    installPlugin?: () => Promise<ProviderPluginInstallation | null>;
  } = {},
) {
  const builtinPlugin=(id:string,name:string)=>({
    key:id+"@builtin",pluginId:id,extensionId:"main",kind:"provider.tools" as const,
    displayName:name,description:name+" Provider plugin",packageVersion:"1.0.0",
    sourceKind:"builtin" as const,status:"enabled" as const,enabled:true,
    capabilities:[],usageCount:0,configVersion:1,protocolVersion:1,
  });
  const resolvedClient = {
    listIntegrationOfferings:vi.fn().mockResolvedValue({integrations:[]}),
    listToolSuites:vi.fn().mockResolvedValue({suites:[]}),
    listPluginCatalogue:vi.fn().mockResolvedValue({plugins:[
      builtinPlugin("io.mender.provider.mcp","MCP"),
      builtinPlugin("io.mender.provider.model-tools","Model Tools"),
      builtinPlugin("io.mender.provider.openapi","OpenAPI"),
      {...builtinPlugin("io.mender.tunnel.openai","OpenAI Secure MCP Tunnel"),
        kind:"tunnel.transport" as const},
    ]}),
    listProviderPlugins: vi.fn().mockResolvedValue({
      plugins: [
        {
          pluginId: "io.mender.provider.mcp",
          version: "1.0.0",
          packageVersion: "1.0.0",
          displayName: "MCP",
          description: "Model Context Protocol provider",
          configVersion: 1,
          sourceKind: "builtin",
          capabilities: [],
          enabled: true,
        },
        {
          pluginId: "io.mender.provider.model-tools",
          version: "1.0.0",
          packageVersion: "1.0.0",
          displayName: "Model Tools",
          description: "Bridge provider-hosted model tools into governed Mender Tools",
          configVersion: 1,
          sourceKind: "builtin",
          capabilities: [],
          enabled: true,
        },
        {
          pluginId: "io.mender.provider.openapi",
          version: "1.0.0",
          packageVersion: "1.0.0",
          displayName: "OpenAPI",
          description: "Expose OpenAPI operations as governed Tools",
          configVersion: 1,
          sourceKind: "builtin",
          capabilities: [],
          enabled: true,
        },
      ],
      issues: [],
    }),
    listProviderPluginInstallations: vi
      .fn()
      .mockResolvedValue({ installations: [] }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listTunnels: vi.fn().mockResolvedValue({ tunnels: [] }),
    listSandboxes: vi.fn().mockResolvedValue({ sandboxes: [] }),
    getSandbox: vi.fn(),
    createLocalSandbox: vi.fn(),
    getSandboxBackends: vi.fn().mockResolvedValue({ dockerAvailable: false, dockerStatus: "unavailable", daytonaConfigured: false, daytonaStatus: "credential required" }),
    createDockerSandbox: vi.fn(),
    reconcileDockerSandbox: vi.fn(),
    ...client,
  } as MenderClient;
  return render(
    <MemoryRouter initialEntries={[path]}>
      <MenderApp
        client={resolvedClient}
        {...(options.installPlugin === undefined
          ? {}
          : { installPlugin: options.installPlugin })}
      />
    </MemoryRouter>,
  );
}
