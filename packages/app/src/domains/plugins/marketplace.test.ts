import { describe, expect, it, vi } from "vitest";
import type { MenderClient, PluginCatalogueEntry } from "@mender/sdk";
import {
  humanizePluginId,
  marketplaceCursorPage,
  pluginCatalogueToMarketplace,
  loadMarketplacePlugins,
  removeMarketplacePlugin,
  setMarketplacePluginEnabled,
} from "./marketplace";

const id = "pinst_0123456789abcdef0123456789abcdef";
function entry(kind:PluginCatalogueEntry["kind"],overrides:Partial<PluginCatalogueEntry>={}):PluginCatalogueEntry{
  const pluginId = kind==="provider.tools"?"dev.example.provider":
    kind==="sandbox.backend"?"dev.example.sandbox":"dev.example.tunnel";
  return {
    key:pluginId+"@"+id,pluginId,extensionId:"main",kind,
    displayName:"Example "+kind,description:"A real typed extension",
    packageVersion:"1.0.0",sourceKind:"local",status:"enabled",enabled:true,
    installationId:id,digest:"sha256"+"a".repeat(64),installedAt:"2026-09-30T00:00:00Z",
    configVersion:1,protocolVersion:1,capabilities:["file.read"],usageCount:0,
    ...overrides,
  };
}

describe("canonical typed Marketplace",()=>{
  it("shows all three extension types from one authority and routes exact creation IDs",()=>{
    const models=pluginCatalogueToMarketplace([
      entry("provider.tools"),entry("sandbox.backend"),entry("tunnel.transport"),
    ]);
    expect(models).toHaveLength(3);
    expect(models.find((x)=>x.kind==="provider")?.createAction?.href).toBe(
      "/providers?create=1&pluginId=dev.example.provider&installationId="+id);
    expect(models.find((x)=>x.kind==="sandbox")?.createAction?.href).toBe(
      "/sandboxes/new/external?create=1&pluginId=dev.example.sandbox&installationId="+id);
    expect(models.find((x)=>x.kind==="tunnel")?.createAction?.href).toBe(
      "/tunnels?create=1&pluginId=dev.example.tunnel&installationId="+id);
  });

  it("prevents enable/remove of unavailable or referenced packages",()=>{
    const models=pluginCatalogueToMarketplace([
      entry("sandbox.backend",{enabled:false,status:"disabled",usageCount:2}),
      entry("tunnel.transport",{enabled:true,usageCount:1}),
      entry("provider.tools",{enabled:true,issue:"verification failed"}),
    ]);
    expect(models.find(x=>x.kind==="sandbox")?.actions).toEqual({enable:true,disable:false,remove:false});
    expect(models.find(x=>x.kind==="tunnel")?.actions).toEqual({enable:false,disable:false,remove:false});
    expect(models.find(x=>x.kind==="provider")?.createAction).toBeUndefined();
    expect(models.find(x=>x.kind==="provider")?.status).toBe("degraded");
  });

  it("keeps builtin entries inert and prevents unsafe install management",()=>{
    const builtin=entry("tunnel.transport",{
      key:"io.mender.tunnel.openai@builtin",pluginId:"io.mender.tunnel.openai",
      sourceKind:"builtin",usageCount:0,
    });
    delete builtin.installationId;
    const model=pluginCatalogueToMarketplace([builtin])[0];
    expect(model?.actions).toEqual({enable:false,disable:false,remove:false});
    expect(model?.createAction?.href).toBe("/tunnels?create=1&pluginId=io.mender.tunnel.openai");
  });

  it("shows the dormant Docker built-in without enabling unsafe creation",()=>{
    const builtin=entry("sandbox.backend",{
      key:"io.mender.sandbox.docker@builtin",
      pluginId:"io.mender.sandbox.docker",
      displayName:"Docker Engine",
      sourceKind:"builtin",
      status:"unavailable",
      enabled:false,
      usageCount:0,
      issue:"Docker Engine is not configured",
    });
    delete builtin.installationId;
    const result=pluginCatalogueToMarketplace([builtin])[0]!;
    expect(result.status).toBe("unavailable");
    expect(result.issue).toEqual({code:"runtime_unavailable",message:"Docker Engine is not configured"});
    expect(result.createAction).toBeUndefined();
    expect(result.actions).toEqual({enable:false,disable:false,remove:false});
  });

  it("maps all Sandbox picker entries into honest Marketplace actions",()=>{
    const builtin=(pluginId:string, overrides:Partial<PluginCatalogueEntry>={}):PluginCatalogueEntry=>{
      const base=entry("sandbox.backend",{
        key:pluginId+"@builtin",pluginId,sourceKind:"builtin",
        displayName:pluginId.split(".").at(-1)||pluginId,
        enabled:true,status:"enabled",usageCount:0,
      });
      delete base.installationId;
      return {...base,...overrides};
    };
    const preview=builtin("io.mender.sandbox.e2b",{
      key:"io.mender.sandbox.e2b@planned",displayName:"E2B",
      sourceKind:"planned",status:"planned",enabled:false,
      packageVersion:"",capabilities:[],
    });
    delete preview.configVersion;
    delete preview.protocolVersion;
    const models=pluginCatalogueToMarketplace([
      builtin("io.mender.sandbox.local-workspace",{displayName:"Local Workspace"}),
      builtin("io.mender.sandbox.docker",{displayName:"Docker Engine"}),
      builtin("io.mender.sandbox.daytona",{displayName:"Daytona",enabled:false,
        status:"unavailable",issue:"Experimental Daytona disabled"}),
      preview,
    ]);
    expect(models).toHaveLength(4);
    expect(models.find(x=>x.key==="io.mender.sandbox.local-workspace@builtin")?.createAction?.href)
      .toBe("/sandboxes/new?create=1&pluginId=io.mender.sandbox.local-workspace");
    expect(models.find(x=>x.key==="io.mender.sandbox.docker@builtin")?.createAction?.href)
      .toBe("/sandboxes/new?create=1&pluginId=io.mender.sandbox.docker");
    expect(models.find(x=>x.key==="io.mender.sandbox.daytona@builtin")?.createAction).toBeUndefined();
    const planned=models.find(x=>x.key==="io.mender.sandbox.e2b@planned");
    expect(planned).toMatchObject({
      status:"planned",source:"planned",sourceLabel:"Roadmap",version:"Not released",
      enabled:false,capabilities:[],actions:{enable:false,disable:false,remove:false},
    });
    expect(planned?.createAction).toBeUndefined();
    expect(planned?.actionRef).toBeUndefined();
    expect(planned?.issue).toBeUndefined();
    const enabledDaytona=pluginCatalogueToMarketplace([
      builtin("io.mender.sandbox.daytona",{displayName:"Daytona (experimental)"}),
    ])[0]!;
    expect(enabledDaytona.createAction?.href)
      .toBe("/sandboxes/new?create=1&pluginId=io.mender.sandbox.daytona");
  });

  it("uses stable cursor pagination",()=>{
    const models=pluginCatalogueToMarketplace(
      Array.from({length:13},(_,i)=>entry("provider.tools",{
        pluginId:"dev.example.plugin-"+String(i).padStart(2,"0"),
        displayName:"Plugin "+String(i).padStart(2,"0"),
        key:"dev.example.plugin-"+String(i).padStart(2,"0")+"@builtin",
      })));
    const page=marketplaceCursorPage(models,null,12);
    expect(page.items).toHaveLength(12);
    expect(marketplaceCursorPage(models,page.nextCursor,12).items).toHaveLength(1);
  });

  it("loads exactly one canonical catalogue and dispatches generic installation actions",async()=>{
    const listPluginCatalogue=vi.fn().mockResolvedValue({plugins:[entry("sandbox.backend")]});
    const disablePluginInstallation=vi.fn().mockResolvedValue({});
    const deletePluginInstallation=vi.fn().mockResolvedValue(undefined);
    const client={
      listPluginCatalogue,disablePluginInstallation,deletePluginInstallation,
    } as unknown as MenderClient;
    const result=await loadMarketplacePlugins(client);
    expect(listPluginCatalogue).toHaveBeenCalledTimes(1);
    await setMarketplacePluginEnabled(client,result[0]!,false);
    await removeMarketplacePlugin(client,result[0]!);
    expect(disablePluginInstallation).toHaveBeenCalledWith(id);
    expect(deletePluginInstallation).toHaveBeenCalledWith(id);
  });

  it("humanizes unknown ids without exposing full technical identifier",()=>{
    expect(humanizePluginId("dev.example.audit-exporter")).toBe("Audit Exporter");
  });
});
