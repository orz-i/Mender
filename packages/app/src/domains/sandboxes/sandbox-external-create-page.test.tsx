import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ManagementClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ExternalSandboxCreatePage } from "./sandbox-external-create-page";

const pluginId = "dev.example.dynamic";
const installationId = "pinst_0123456789abcdef0123456789abcdef";

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

describe("External Sandbox configuration", () => {
  it("uses only remaining configuration when reviewing and creating an external Sandbox", async () => {
    const sandboxId="sbx_0123456789abcdef0123456789abcdef";
    const createExternalSandbox=vi.fn().mockResolvedValue({sandboxId});
    const client={
      listPluginCatalogue:vi.fn().mockResolvedValue({plugins:[{
        key:pluginId+"@"+installationId,pluginId,installationId,
        extensionId:"main",kind:"sandbox.backend",marketplaceAction:"create.sandbox",
        displayName:"Optional Sandbox",description:"Fixture",packageVersion:"1.0.0",
        sourceKind:"local",status:"enabled",enabled:true,capabilities:[],
        usageCount:0,configSchema:{type:"object",properties:{
          region:{type:"string",default:"local"},
          options:{type:"object",required:["token"],properties:{token:{type:"string"}}},
        }},
      }]}),
      createExternalSandbox,
    } as unknown as ManagementClient;
    render(<MemoryRouter initialEntries={[`/sandboxes/new/external?pluginId=${pluginId}&installationId=${installationId}`]}>
      <ExternalSandboxCreatePage client={client}/>
    </MemoryRouter>);
    fireEvent.change(await screen.findByRole("textbox",{name:"Sandbox name"}),{target:{value:"QA Sandbox"}});
    const review=screen.getByRole<HTMLButtonElement>("button",{name:"Review"});
    expect(review.disabled).toBe(false);
    fireEvent.click(screen.getByRole("button",{name:"Add Options"}));
    expect(review.disabled).toBe(true);
    fireEvent.change(screen.getByRole("textbox",{name:"Token"}),{target:{value:"fixture"}});
    expect(review.disabled).toBe(false);
    fireEvent.click(screen.getByRole("button",{name:"Remove Options"}));
    expect(review.disabled).toBe(false);
    expect(createExternalSandbox).not.toHaveBeenCalled();
    fireEvent.click(review);
    expect(await screen.findByText(installationId)).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Create sandbox"}));
    await waitFor(()=>expect(createExternalSandbox).toHaveBeenCalledWith({
      name:"QA Sandbox",installationId,config:{region:"local"},
    }));
  });

  it("prevents Sandbox Review from using the last valid JSON array while an invalid draft is visible", async () => {
    const sandboxId = "sbx_0123456789abcdef0123456789abcdef";
    const createExternalSandbox = vi.fn().mockResolvedValue({ sandboxId });
    const client = {
      listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: [{
        key: pluginId+"@"+installationId, pluginId, installationId,
        extensionId: "main", kind: "sandbox.backend", marketplaceAction: "create.sandbox",
        displayName: "Dynamic Sandbox", description: "Fixture", packageVersion: "1.0.0",
        sourceKind: "local", status: "enabled", enabled: true, capabilities: ["file.read"],
        usageCount: 0, configSchema: {type:"object",properties: {records: {type:"array",default:[1]}}}
      }] }),
      createExternalSandbox,
    } as unknown as ManagementClient;
    render(<MemoryRouter initialEntries={[`/sandboxes/new/external?pluginId=${pluginId}&installationId=${installationId}`]}>
      <ExternalSandboxCreatePage client={client} />
    </MemoryRouter>);
    const name = await screen.findByRole("textbox", { name: "Sandbox name" });
    fireEvent.change(name, { target: { value: "Fixture Sandbox" } });
    const records = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Records" });
    const review = screen.getByRole<HTMLButtonElement>("button", { name: "Review" });
    expect(review.disabled).toBe(false);
    fireEvent.change(records, { target: { value: "[1," } });
    expect(review.disabled).toBe(true);
    expect(createExternalSandbox).not.toHaveBeenCalled();
    fireEvent.change(records, { target: { value: "[1,2]" } });
    expect(review.disabled).toBe(false);
    fireEvent.click(review);
    expect(await screen.findByText(installationId)).toBeTruthy();
    expect(createExternalSandbox).not.toHaveBeenCalled();
  });
});
