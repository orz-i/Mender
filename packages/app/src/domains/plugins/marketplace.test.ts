import { describe, expect, it, vi } from "vitest";
import type { IntegrationOffering, MenderClient, PluginCatalogueEntry } from "@mender/sdk";
import {
  humanizePluginId,
  marketplaceCursorPage,
  pluginCatalogueToMarketplace,
  integrationOfferingsToMarketplace,
  loadMarketplacePlugins,
  removeMarketplacePlugin,
  setMarketplacePluginEnabled,
} from "./marketplace";

const id = "pinst_0123456789abcdef0123456789abcdef";
function entry(kind:PluginCatalogueEntry["kind"],overrides:Partial<PluginCatalogueEntry>={}):PluginCatalogueEntry{
  const pluginId=kind==="provider.tools"?"dev.example.provider":
    kind==="sandbox.backend"?"dev.example.sandbox":
    kind==="agent.backend"?"dev.example.agent":"dev.example.tunnel";
  return {
    key:pluginId+"@"+id,pluginId,extensionId:"main",kind,
    marketplaceAction:kind==="provider.tools"?"create.provider":
      kind==="sandbox.backend"?"create.sandbox":
        kind==="tunnel.transport"?"create.tunnel":"none",
    displayName:"Example "+kind,description:"A verified native extension",
    packageVersion:"1.0.0",sourceKind:"local",status:"enabled",enabled:true,
    installationId:id,digest:"sha256:"+"a".repeat(64),installedAt:"2026-09-30T00:00:00Z",
    configVersion:1,protocolVersion:1,capabilities:["file.read"],usageCount:0,
    ...overrides,
  };
}
function builtin(kind:PluginCatalogueEntry["kind"],pluginId:string,
  action:PluginCatalogueEntry["marketplaceAction"],overrides:Partial<PluginCatalogueEntry>={}):PluginCatalogueEntry{
  const value=entry(kind,{
    pluginId,key:pluginId+"@builtin",sourceKind:"builtin",
    marketplaceAction:action, ...overrides,
  });
  delete value.installationId;
  return value;
}
const anySearch:IntegrationOffering={
  offeringId:"io.mender.integration.anysearch",displayName:"AnySearch",
  description:"Hosted MCP search",kind:"remote-mcp",
  driverPluginId:"io.mender.provider.mcp",transport:"streamable-http",
  endpoint:"https://api.anysearch.com/mcp",
  headers:{"X-Anysearch-Client":"mcp/1.0.0"},defaultNamespace:"anysearch",
  authentication:"optional-bearer",sourceUrl:"https://github.com/anysearch-ai/anysearch-mcp-server",
};

describe("Host-reviewed Marketplace projection",()=>{
  it("never projects private managed facades or unreviewed built-ins",()=>{
    const records=[
      builtin("provider.tools","io.mender.provider.workspace","none"),
      builtin("provider.tools","io.mender.provider.agent-delegation","none"),
      builtin("provider.tools","io.mender.provider.sandbox-jobs","none"),
      builtin("provider.tools","dev.example.unknown","none"),
      entry("provider.tools"),
    ];
    const result=pluginCatalogueToMarketplace(records);
    expect(result.map(x=>x.key)).toEqual(["dev.example.provider@"+id]);
  });

  it("makes MCP, OpenAPI and Model Tools first-class creatable offerings",()=>{
    const models=pluginCatalogueToMarketplace([
      builtin("provider.tools","io.mender.provider.mcp","create.provider"),
      builtin("provider.tools","io.mender.provider.openapi","create.provider"),
      builtin("provider.tools","io.mender.provider.model-tools","create.provider"),
    ]);
    expect(models).toHaveLength(3);
    for(const model of models){
      expect(model.createAction?.href).toBe("/providers?create=1&pluginId="+model.key.split("@")[0]);
      expect(model.actions).toEqual({enable:false,disable:false,remove:false});
    }
  });

  it("routes exact installed Provider, Sandbox and Tunnel IDs",()=>{
    const models=pluginCatalogueToMarketplace([
      entry("provider.tools"),entry("sandbox.backend"),entry("tunnel.transport"),
    ]);
    expect(models.find(x=>x.kind==="provider")?.createAction?.href).toBe(
      "/providers?create=1&pluginId=dev.example.provider&installationId="+id);
    expect(models.find(x=>x.kind==="sandbox")?.createAction?.href).toBe(
      "/sandboxes/new/external?create=1&pluginId=dev.example.sandbox&installationId="+id);
    expect(models.find(x=>x.kind==="tunnel")?.createAction?.href).toBe(
      "/tunnels?create=1&pluginId=dev.example.tunnel&installationId="+id);
  });

  it("refuses inconsistent actions, even when the extension claims attractive capabilities",()=>{
    expect(()=>pluginCatalogueToMarketplace([
      entry("sandbox.backend",{marketplaceAction:"create.provider"}),
    ])).toThrow(/action and plugin extension kind disagree/);
    expect(pluginCatalogueToMarketplace([
      builtin("agent.backend","io.mender.agent.codex","none"),
    ])).toHaveLength(0);
  });

  it("fails closed on unknown native extension kinds",()=>{
    expect(()=>pluginCatalogueToMarketplace([
      entry("provider.tools",{
        kind:"provider.future" as PluginCatalogueEntry["kind"],
        marketplaceAction:"none",
      }),
    ])).toThrow(/Unsupported plugin extension kind/);
  });

  it("keeps native Agent packages visible for diagnosis only, not create or unsupported management",()=>{
    const models=pluginCatalogueToMarketplace([
      builtin("agent.backend","io.mender.agent.grok","create.agent"),
      builtin("agent.backend","io.mender.agent.codex","create.agent",{
        status:"unavailable",enabled:false,issue:"codex-acp missing",
      }),
      entry("agent.backend"),
    ]);
    expect(models.find(x=>x.key==="io.mender.agent.grok@builtin")?.createAction?.href)
      .toBe("/agents/new?create=1&pluginId=io.mender.agent.grok");
    expect(models.find(x=>x.key==="io.mender.agent.codex@builtin")?.createAction).toBeUndefined();
    expect(models.find(x=>x.key==="dev.example.agent@"+id)).toMatchObject({
      actions:{enable:false,disable:false,remove:false},
    });
    expect(models.find(x=>x.key==="dev.example.agent@"+id)?.createAction).toBeUndefined();
  });

  it("preserves disabled, unavailable, invalid and referenced installation diagnostics",()=>{
    const models=pluginCatalogueToMarketplace([
      entry("sandbox.backend",{enabled:false,status:"disabled",usageCount:2}),
      entry("tunnel.transport",{enabled:true,usageCount:1}),
      entry("provider.tools",{enabled:false,status:"invalid",issue:"verification failed",
        marketplaceAction:"none"}),
      builtin("sandbox.backend","io.mender.sandbox.docker","create.sandbox",{
        enabled:false,status:"unavailable",issue:"Docker Engine not configured",
      }),
    ]);
    expect(models.find(x=>x.kind==="sandbox"&&x.source==="local")?.actions)
      .toEqual({enable:true,disable:false,remove:false});
    expect(models.find(x=>x.kind==="tunnel")?.actions)
      .toEqual({enable:false,disable:false,remove:false});
    const invalid=models.find(x=>x.kind==="provider");
    expect(invalid).toMatchObject({status:"degraded",
      actions:{enable:false,disable:false,remove:true}});
    expect(invalid?.createAction).toBeUndefined();
    const docker=models.find(x=>x.key==="io.mender.sandbox.docker@builtin");
    expect(docker?.createAction).toBeUndefined();
    expect(docker?.issue?.code).toBe("runtime_unavailable");
  });

  it("keeps approved Sandbox options and rejects a missing implementation",()=>{
    const models=pluginCatalogueToMarketplace([
      builtin("sandbox.backend","io.mender.sandbox.local-workspace","create.sandbox"),
      builtin("sandbox.backend","io.mender.sandbox.ssh-remote","create.sandbox"),
      builtin("sandbox.backend","io.mender.sandbox.daytona","create.sandbox",{enabled:false,status:"unavailable"}),
      builtin("sandbox.backend","io.mender.sandbox.e2b","none"),
    ]);
    expect(models).toHaveLength(3);
    expect(models.find(x=>x.key==="io.mender.sandbox.e2b@builtin")).toBeUndefined();
    expect(models.find(x=>x.key==="io.mender.sandbox.local-workspace@builtin")?.createAction?.href)
      .toBe("/sandboxes/new?create=1&pluginId=io.mender.sandbox.local-workspace");
  });

  it("offers AnySearch as a connection recipe, not another Provider installation",()=>{
    const [item]=integrationOfferingsToMarketplace([anySearch]);
    expect(item).toMatchObject({kind:"integration",source:"curated",status:"available",
      actions:{enable:false,disable:false,remove:false},
      createAction:{label:"Connect",href:"/providers?create=1&offeringId=io.mender.integration.anysearch"},
    });
    expect(item?.technicalDetails.find(x=>x.label==="Endpoint")?.value).toBe("https://api.anysearch.com/mcp");
    expect(()=>integrationOfferingsToMarketplace([
      {...anySearch,kind:"cloud-browser" as IntegrationOffering["kind"]},
    ])).toThrow(/Unsupported reviewed/);
  });

  it("supports stable cursor pagination",()=>{
    const models=pluginCatalogueToMarketplace(
      Array.from({length:13},(_,i)=>entry("provider.tools",{
        pluginId:"dev.example.plugin-"+String(i).padStart(2,"0"),
        displayName:"Plugin "+String(i).padStart(2,"0"),
        key:"dev.example.plugin-"+String(i).padStart(2,"0")+"@"+id,
      })));
    const first=marketplaceCursorPage(models,null,12);
    expect(first.items).toHaveLength(12);
    expect(marketplaceCursorPage(models,first.nextCursor,12).items).toHaveLength(1);
  });

  it("reads one catalogue and curated integrations without querying Tool Suites for sale",async()=>{
    const listPluginCatalogue=vi.fn().mockResolvedValue({plugins:[entry("sandbox.backend")]});
    const listIntegrationOfferings=vi.fn().mockResolvedValue({integrations:[anySearch]});
    const listToolSuites=vi.fn();
    const disablePluginInstallation=vi.fn().mockResolvedValue({});
    const deletePluginInstallation=vi.fn().mockResolvedValue(undefined);
    const client={listPluginCatalogue,listIntegrationOfferings,listToolSuites,
      disablePluginInstallation,deletePluginInstallation} as unknown as MenderClient;
    const result=await loadMarketplacePlugins(client);
    expect(result).toHaveLength(2);
    expect(listPluginCatalogue).toHaveBeenCalledOnce();
    expect(listIntegrationOfferings).toHaveBeenCalledOnce();
    expect(listToolSuites).not.toHaveBeenCalled();
    const sandbox=result.find(x=>x.kind==="sandbox")!;
    await setMarketplacePluginEnabled(client,sandbox,false);
    await removeMarketplacePlugin(client,sandbox);
    expect(disablePluginInstallation).toHaveBeenCalledWith(id);
    expect(deletePluginInstallation).toHaveBeenCalledWith(id);
  });
  it("humanizes technical names only as fallback",()=>{
    expect(humanizePluginId("dev.example.audit-exporter")).toBe("Audit Exporter");
  });
});
