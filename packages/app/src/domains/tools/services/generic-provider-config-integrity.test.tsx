import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ManagementClient, Provider, ProviderPlugin } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../../i18n/i18n";
import { ToolsPage } from "./tool-services-page";

const pluginId = "dev.example.dynamic";
const installationId = "pinst_0123456789abcdef0123456789abcdef";
const targetInstallationId = "pinst_1123456789abcdef0123456789abcdef";

function plugin(configSchema: ProviderPlugin["configSchema"], installation = installationId): ProviderPlugin {
  return {
    pluginId, installationId: installation, sourceKind: "external",
    displayName: "Dynamic Integration", description: "Example",
    version: "1.0.0", packageVersion: installation === installationId ? "1.0.0" : "2.0.0",
    configVersion: 1, protocolVersion: 1, enabled: true, capabilities: [],
    ...(configSchema === undefined ? {} : { configSchema }),
  };
}

const configuredProvider: Provider = {
  providerId: "prov_0123456789abcdef0123456789abcdef",
  namespace: "dynamic-service", displayName: "Dynamic Service", description: "",
  createdAt: "2026-10-21T00:00:00Z", updatedAt: "2026-10-21T00:00:00Z",
  transport: "unconfigured", toolCount: 0,
  pluginId, installationId, configVersion: 1,
  config: { records: [1], retries: 2 },
};

function mountProviders(plugins: ProviderPlugin[], providers: Provider[], overrides: Record<string, unknown> = {}, url = "/tools") {
  const createProvider = vi.fn().mockResolvedValue({ provider: configuredProvider, sync: { created: 1, updated: 0, removed: 0, total: 1 } });
  const updateProvider = vi.fn().mockResolvedValue({ provider: configuredProvider, sync: { created: 0, updated: 1, removed: 0, total: 1 } });
  const rebindProvider = vi.fn().mockResolvedValue({ provider: configuredProvider, sync: { created: 0, updated: 1, removed: 0, total: 1 } });
  const client = {
    listProviderPlugins: vi.fn().mockResolvedValue({ plugins, issues: [] }),
    listProviders: vi.fn().mockResolvedValue({ providers }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    createProvider, updateProvider, rebindProvider,
    ...overrides,
  } as unknown as ManagementClient;
  render(<MemoryRouter initialEntries={[url]}><ToolsPage client={client}/></MemoryRouter>);
  return { createProvider, updateProvider, rebindProvider };
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

describe("Provider JSON editor submission integrity", () => {
  it("submits an omitted optional group after adding then removing it from Provider creation", async () => {
    const optionalSchema = { type:"object", additionalProperties:false, properties:{
      advanced:{ type:"object", required:["token"], additionalProperties:false,
        properties:{token:{type:"string"}} },
    }};
    const {createProvider} = mountProviders(
      [plugin(optionalSchema)], [],
      {}, `/tools?create=1&pluginId=${pluginId}&installationId=${installationId}`,
    );
    const dialog = await screen.findByRole("dialog", {name:"Add Dynamic Integration · 1.0.0 provider"});
    fireEvent.change(within(dialog).getByRole("textbox",{name:"Namespace"}),{target:{value:"optional-service"}});
    fireEvent.change(within(dialog).getByRole("textbox",{name:"Display name"}),{target:{value:"Optional Service"}});
    const save=within(dialog).getByRole<HTMLButtonElement>("button",{name:"Add provider"});
    expect(save.disabled).toBe(false);
    expect(within(dialog).queryByRole("textbox",{name:"Token"})).toBeNull();
    fireEvent.click(within(dialog).getByRole("button",{name:"Add Advanced"}));
    expect(save.disabled).toBe(true);
    fireEvent.change(within(dialog).getByRole("textbox",{name:"Token"}),{target:{value:"fixture-only"}});
    expect(save.disabled).toBe(false);
    fireEvent.click(within(dialog).getByRole("button",{name:"Remove Advanced"}));
    expect(within(dialog).queryByRole("textbox",{name:"Token"})).toBeNull();
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(()=>expect(createProvider).toHaveBeenCalledWith(expect.objectContaining({config:{}})));
  });

  it("removes a persisted optional object without changing the Provider identity", async () => {
    const optionalSchema={type:"object",additionalProperties:false,properties:{
      advanced:{type:"object",required:["token"],properties:{token:{type:"string"}}},
    }};
    const persisted={...configuredProvider,config:{advanced:{token:"from-provider"}}};
    const updateProvider=vi.fn().mockResolvedValue({provider:persisted,sync:{created:0,updated:1,removed:0,total:1}});
    mountProviders([plugin(optionalSchema)],[persisted],{updateProvider});
    fireEvent.click(await screen.findByRole("button",{name:"Actions: Dynamic Service"}));
    fireEvent.click(await screen.findByRole("button",{name:"Edit Dynamic Service"}));
    const dialog=await screen.findByRole("dialog",{name:"Edit provider"});
    expect(within(dialog).getByRole<HTMLInputElement>("textbox",{name:"Token"}).value).toBe("from-provider");
    fireEvent.click(within(dialog).getByRole("button",{name:"Remove Advanced"}));
    expect(within(dialog).getByRole<HTMLButtonElement>("button",{name:"Save changes"}).disabled).toBe(false);
    fireEvent.click(within(dialog).getByRole("button",{name:"Save changes"}));
    await waitFor(()=>expect(updateProvider).toHaveBeenCalledWith(
      configuredProvider.providerId,expect.objectContaining({config:{}}),
    ));
  });

  it("blocks Provider creation with an invalid raw object even when the last valid object satisfies its schema", async () => {
    const { createProvider } = mountProviders(
      [plugin(true)], [],
      {}, `/tools?create=1&pluginId=${pluginId}&installationId=${installationId}`,
    );
    const dialog = await screen.findByRole("dialog", { name: "Add Dynamic Integration · 1.0.0 provider" });
    const installationDetails = within(dialog).getByText("Installation details").closest("details")!;
    expect(installationDetails.hasAttribute("open")).toBe(false);
    fireEvent.click(installationDetails.querySelector("summary")!);
    expect(installationDetails.hasAttribute("open")).toBe(true);
    expect(within(installationDetails).getByText(installationId)).toBeTruthy();
    fireEvent.click(installationDetails.querySelector("summary")!);
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Namespace" }), { target: { value: "dynamic" } });
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Display name" }), { target: { value: "Dynamic" } });
    const save = within(dialog).getByRole<HTMLButtonElement>("button", { name: "Add provider" });
    const json = within(dialog).getByRole<HTMLTextAreaElement>("textbox", { name: "Provider configuration" });
    expect(save.disabled).toBe(false);
    fireEvent.change(json, { target: { value: '{"retry":' } });
    expect(save.disabled).toBe(true);
    expect(json.getAttribute("aria-invalid")).toBe("true");
    expect(within(dialog).getByRole("alert").textContent).toContain("Enter valid JSON.");
    expect(createProvider).not.toHaveBeenCalled();

    fireEvent.change(json, { target: { value: '{"retry":3}' } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() => expect(createProvider).toHaveBeenCalledWith(expect.objectContaining({config: {retry:3}})));
  });

  it("does not create a Provider using a previous numeric value while an incomplete number is visible", async () => {
    const { createProvider } = mountProviders(
      [plugin({
        type: "object",
        properties: { attempts: { type: "integer", minimum: 1, maximum: 10, default: 3 } },
      })], [],
      {}, `/tools?create=1&pluginId=${pluginId}&installationId=${installationId}`,
    );
    const dialog = await screen.findByRole("dialog", { name: "Add Dynamic Integration · 1.0.0 provider" });
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Namespace" }), { target: { value: "number-guard" } });
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Display name" }), { target: { value: "Guard" } });
    const attempts = within(dialog).getByRole<HTMLInputElement>("textbox", { name: "Attempts" });
    const save = within(dialog).getByRole<HTMLButtonElement>("button", { name: "Add provider" });
    expect(attempts.value).toBe("3");
    expect(save.disabled).toBe(false);
    fireEvent.change(attempts, { target: { value: "1e" } });
    expect(attempts.value).toBe("1e");
    expect(save.disabled).toBe(true);
    expect(within(dialog).getByRole("alert").textContent).toContain("Enter a valid number.");
    expect(createProvider).not.toHaveBeenCalled();
    fireEvent.change(attempts, { target: { value: "4" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() => expect(createProvider).toHaveBeenCalledWith(expect.objectContaining({
      config: { attempts: 4 },
    })));
  });

  it("keeps an edit form open with an inline failure and never submits an invalid array draft", async () => {
    const updateProvider = vi.fn().mockRejectedValueOnce(new Error("Fixture provider update failed"))
      .mockResolvedValueOnce({ provider: configuredProvider, sync: { created: 0, updated: 1, removed: 0, total: 1 } });
    mountProviders([plugin({ type:"object", properties: { records: {type:"array"} } })], [configuredProvider], { updateProvider });
    const actions = await screen.findByRole("button", { name: "Actions: Dynamic Service" });
    fireEvent.click(actions);
    fireEvent.click(await screen.findByRole("button", { name: "Edit Dynamic Service" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit provider" });
    const records = within(dialog).getByRole<HTMLTextAreaElement>("textbox", { name: "Records" });
    const save = within(dialog).getByRole<HTMLButtonElement>("button", { name: "Save changes" });
    fireEvent.change(records, { target: { value: "[1," } });
    expect(save.disabled).toBe(true);
    expect(updateProvider).not.toHaveBeenCalled();

    fireEvent.change(records, { target: { value: "[1,3]" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect((await within(dialog).findByRole("alert")).textContent).toContain("Fixture provider update failed");
    expect(records.value).toBe("[1,3]");
    expect(screen.getByRole("dialog", { name: "Edit provider" })).toBeTruthy();
    fireEvent.click(save);
    await waitFor(() => expect(updateProvider).toHaveBeenCalledTimes(2));
  });

  it("blocks Provider Rebind when target configuration has an invalid local JSON draft", async () => {
    const p = plugin({ type:"object", properties: {records: {type:"array"} } });
    const next = plugin(p.configSchema, targetInstallationId);
    const rebindProvider = vi.fn().mockRejectedValueOnce(new Error("Fixture rebind failed"))
      .mockResolvedValueOnce({provider:configuredProvider});
    mountProviders([p, next], [configuredProvider], { rebindProvider });
    fireEvent.click(await screen.findByRole("button", { name: "Actions: Dynamic Service" }));
    fireEvent.click(await screen.findByRole("button", { name: "Rebind Dynamic Service" }));
    const dialog = await screen.findByRole("dialog", {name:"Rebind Provider installation"});
    expect(dialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(dialog.querySelector('[data-slot="dialog-body"]')).not.toBeNull();
    expect(within(dialog).getByRole<HTMLSelectElement>("combobox", {name:"Target Provider plugin installation"}).value).toBe(targetInstallationId);
    const records = within(dialog).getByRole<HTMLTextAreaElement>("textbox", { name: "Records" });
    const save = within(dialog).getByRole<HTMLButtonElement>("button", { name: "Rebind installation" });
    fireEvent.change(records, { target: { value: "[1," } });
    expect(save.disabled).toBe(true);
    expect(rebindProvider).not.toHaveBeenCalled();
    fireEvent.change(records, { target: { value: "[2]" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect((await within(dialog).findByRole("alert")).textContent).toContain("Fixture rebind failed");
    expect(records.value).toBe("[2]");
    fireEvent.click(save);
    await waitFor(() => expect(rebindProvider).toHaveBeenCalledTimes(2));
    expect(rebindProvider).toHaveBeenLastCalledWith(configuredProvider.providerId, {
      installationId: targetInstallationId,
      config: { records: [2], retries: 2 },
    });
  });

  it("discards only the old installation's invalid JSON draft when Rebind target changes", async () => {
    const schema = { type:"object", properties: { records: { type:"array" } } };
    const nextInstallation = "pinst_22222222222222222222222222222222";
    const rebindProvider = vi.fn();
    mountProviders(
      [plugin(schema), plugin(schema, targetInstallationId), plugin(schema, nextInstallation)],
      [configuredProvider],
      { rebindProvider },
    );
    fireEvent.click(await screen.findByRole("button", { name: "Actions: Dynamic Service" }));
    fireEvent.click(await screen.findByRole("button", { name: "Rebind Dynamic Service" }));
    const dialog = await screen.findByRole("dialog", { name:"Rebind Provider installation" });
    const picker = within(dialog).getByRole<HTMLSelectElement>("combobox", { name: "Target Provider plugin installation" });
    const records = within(dialog).getByRole<HTMLTextAreaElement>("textbox", { name: "Records" });
    const save = within(dialog).getByRole<HTMLButtonElement>("button", { name: "Rebind installation" });
    expect(picker.value).toBe(targetInstallationId);
    fireEvent.change(records, { target: { value: "[1," } });
    expect(save.disabled).toBe(true);
    fireEvent.change(picker, { target: { value: nextInstallation } });
    expect(picker.value).toBe(nextInstallation);
    expect(within(dialog).getByRole<HTMLTextAreaElement>("textbox", { name: "Records" }).value).toBe("[\n  1\n]");
    expect(save.disabled).toBe(false);
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(rebindProvider).not.toHaveBeenCalled();
  });
});
