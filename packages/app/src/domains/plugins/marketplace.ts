import type {
  MenderClient,
  IntegrationOffering,
  PluginCatalogueEntry,
  ProviderPluginInstallation,
} from "@mender/sdk";

export type MarketplacePluginKind = string;
export type MarketplacePluginSource = string;
export type MarketplacePluginStatus =
  | "enabled"
  | "available"
  | "disabled"
  | "invalid"
  | "degraded"
  | "unavailable";

export interface MarketplacePluginActions {
  enable: boolean;
  disable: boolean;
  remove: boolean;
}

export interface MarketplaceTechnicalDetail {
  label: string;
  value: string;
  mono?: boolean;
  format?: "text" | "dateTime";
}

export interface MarketplacePluginIssue {
  code: string;
  message: string;
}

export interface MarketplaceCreateAction {
  label: string;
  href: string;
}

export interface MarketplacePlugin {
  key: string;
  kind: MarketplacePluginKind;
  kindLabel: string;
  displayName: string;
  description: string;
  source: MarketplacePluginSource;
  sourceLabel: string;
  version: string;
  status: MarketplacePluginStatus;
  enabled: boolean;
  capabilities: string[];
  actions: MarketplacePluginActions;
  technicalDetails: MarketplaceTechnicalDetail[];
  createAction?: MarketplaceCreateAction;
  actionRef?: string;
  issue?: MarketplacePluginIssue;
}

export interface MarketplaceInstaller {
  key: string;
  label: string;
  pendingLabel: string;
  install: () => Promise<boolean>;
}

export function pluginInstaller(
  install:()=>Promise<ProviderPluginInstallation|null>,
):MarketplaceInstaller{
  return {key:"local-package",label:"Install plugin",pendingLabel:"Installing…",
    async install(){return (await install())!==null;}};
}

export function pluginCatalogueToMarketplace(entries:PluginCatalogueEntry[]):MarketplacePlugin[]{
  // Marketplace eligibility comes from the Host, not from capability claims
  // or a guessed route. Native installations remain for repair/removal.
  return entries.filter(entry=>entry.marketplaceAction!=="none" || Boolean(entry.installationId))
    .map((entry):MarketplacePlugin=>{
    const kinds:Record<PluginCatalogueEntry["kind"], "provider"|"sandbox"|"tunnel"|"agent">={
      "provider.tools":"provider",
      "sandbox.backend":"sandbox",
      "tunnel.transport":"tunnel",
      "agent.backend":"agent",
    };
    const kind=kinds[entry.kind];
    if(kind===undefined){throw new Error("Unsupported plugin extension kind");}
    const labels={provider:"Provider plugin",sandbox:"Sandbox backend",
      tunnel:"Tunnel transport",agent:"Agent backend"};
    const enabled=entry.enabled && entry.status==="enabled" && entry.issue===undefined;
    const installationId=entry.installationId;
    const action=entry.marketplaceAction;
    const actionKind=action==="create.provider"?"provider":action==="create.sandbox"?"sandbox":
      action==="create.agent"?"agent":action==="create.tunnel"?"tunnel":null;
    if(actionKind!==null && actionKind!==kind){
      throw new Error("Host Marketplace action and plugin extension kind disagree");
    }
    const params=new URLSearchParams({create:"1",pluginId:entry.pluginId});
    if(action==="create.provider")params.set("view","sources");
    if(installationId)params.set("installationId",installationId);
    const pathname=action==="create.provider"?"/tools":
      action==="create.sandbox"?(installationId?"/sandboxes/new/external":"/sandboxes/new"):
      action==="create.agent"?"/agents/new":action==="create.tunnel"?"/tunnels":null;
    const createAction=enabled && pathname?{
      label:action==="create.provider"?"Add provider":action==="create.sandbox"?"Create sandbox":
        action==="create.agent"?"Add agent":"Add tunnel",
      href:pathname+"?"+params.toString(),
    }:undefined;
    // An installation is managed by its exact ID, not by a creatable route.
    const manageableInstallation=!!installationId && ["provider","sandbox","tunnel"].includes(kind);
    return {
      key:entry.key,kind,kindLabel:labels[kind],
      displayName:entry.displayName||humanizePluginId(entry.pluginId),
      description:entry.description,
      source:entry.sourceKind,sourceLabel:entry.sourceKind==="builtin"?"Built-in":"Local package",
      version:entry.packageVersion,
      status:entry.status==="unavailable"?"unavailable":entry.issue?"degraded":entry.status,
      enabled:entry.enabled,
      capabilities:entry.capabilities,
      actions:manageableInstallation?{
        enable:entry.status==="disabled"&&!entry.issue,
        disable:entry.status==="enabled"&&entry.usageCount===0,
        remove:entry.usageCount===0,
      }:noMarketplaceActions(),
      technicalDetails:[
        {label:"Plugin ID",value:entry.pluginId,mono:true},
        {label:"Extension ID",value:entry.extensionId,mono:true},
        {label:"Installation ID",value:installationId??"Built-in",mono:!!installationId},
        ...(entry.digest?[{label:"Digest",value:entry.digest,mono:true}]:[]),
        ...(entry.installedAt?[{label:"Installed at",value:entry.installedAt,format:"dateTime" as const}]:[]),
        {label:"Used by resources",value:String(entry.usageCount)},
        ...(entry.configVersion?[{label:"Config version",value:String(entry.configVersion)}]:[]),
        ...(entry.protocolVersion?[{label:"Protocol version",value:String(entry.protocolVersion)}]:[]),
      ],
      ...(createAction?{createAction}:{}),
      ...(installationId?{actionRef:installationId}:{}),
      ...(entry.issue?{issue:{code:entry.status==="unavailable"?"runtime_unavailable":"descriptor_unavailable",message:entry.issue}}:{}),
    };
  }).toSorted(compareMarketplacePlugins);
}

export function integrationOfferingsToMarketplace(offerings:IntegrationOffering[]):MarketplacePlugin[] {
  return offerings.map((entry):MarketplacePlugin=>{
    if(entry.kind!=="remote-mcp"||entry.transport!=="streamable-http"||
      entry.driverPluginId!=="io.mender.provider.mcp"||entry.authentication!=="optional-bearer"||
      !entry.offeringId.startsWith("io.mender.integration.")||!entry.endpoint.startsWith("https://")){
      throw new Error("Unsupported reviewed integration offering");
    }
    return {
      key:"integration:"+entry.offeringId,kind:"integration",kindLabel:"Remote MCP integration",
      displayName:entry.displayName,description:entry.description,
      source:"curated",sourceLabel:"Curated",
      version:"Reviewed preset",status:"available",enabled:false,
      capabilities:["MCP","streamable-http"],
      actions:noMarketplaceActions(),
      technicalDetails:[
        {label:"Offering ID",value:entry.offeringId,mono:true},
        {label:"Protocol",value:"MCP · Streamable HTTP"},
        {label:"Driver Plugin",value:entry.driverPluginId,mono:true},
        {label:"Endpoint",value:entry.endpoint,mono:true},
        {label:"Authentication",value:"Optional API Credential"},
        {label:"Source",value:entry.sourceUrl},
      ],
      createAction:{
        label:"Connect",
        href:"/tools?"+new URLSearchParams({view:"sources",create:"1",offeringId:entry.offeringId}).toString(),
      },
    };
  });
}

export async function loadMarketplacePlugins(client:MenderClient){
  const [catalogue,integrations]=await Promise.all([
    client.listPluginCatalogue(),client.listIntegrationOfferings(),
  ]);
  return [...pluginCatalogueToMarketplace(catalogue.plugins),
    ...integrationOfferingsToMarketplace(integrations.integrations)]
    .toSorted(compareMarketplacePlugins);
}

export function marketplaceCursorPage(
  plugins: MarketplacePlugin[],
  cursor: string | null,
  limit = 12,
) {
  const cursorIndex =
    cursor === null
      ? -1
      : plugins.findIndex((plugin) => plugin.key === cursor);
  const startIndex = cursorIndex >= 0 ? cursorIndex + 1 : 0;
  const items = plugins.slice(startIndex, startIndex + limit);
  const hasNext = startIndex + items.length < plugins.length;

  return {
    items,
    from: items.length ? startIndex + 1 : 0,
    to: startIndex + items.length,
    total: plugins.length,
    nextCursor: hasNext ? (items.at(-1)?.key ?? null) : null,
  };
}

export async function setMarketplacePluginEnabled(
  client:MenderClient,plugin:MarketplacePlugin,enabled:boolean,
){
  if(!(enabled?plugin.actions.enable:plugin.actions.disable)){
    throw new Error("Marketplace plugin does not support this state change");
  }
  const id=requireActionRef(plugin);
  if(enabled){await client.enablePluginInstallation(id);}
  else{await client.disablePluginInstallation(id);}
}

export async function removeMarketplacePlugin(
  client:MenderClient,plugin:MarketplacePlugin,
){
  if(!plugin.actions.remove){throw new Error("Marketplace plugin does not support removal");}
  await client.deletePluginInstallation(requireActionRef(plugin));
}

export function humanizePluginId(pluginId: string) {
  const known: Record<string, string> = {
    "io.mender.provider.mcp": "MCP",
    "io.mender.provider.model-tools": "Model Tools",
    "io.mender.provider.openapi": "OpenAPI",
    "io.mender.provider.workspace": "Workspace",
  };
  const knownName = known[pluginId];
  if (knownName !== undefined) {
    return knownName;
  }

  const segment = pluginId.split(".").at(-1) || pluginId;
  return segment
    .split(/[-_]/u)
    .filter(Boolean)
    .map((part) => part.charAt(0).toLocaleUpperCase() + part.slice(1))
    .join(" ");
}

function requireActionRef(plugin: MarketplacePlugin) {
  if (plugin.actionRef === undefined) {
    throw new Error("Marketplace plugin action reference is unavailable");
  }
  return plugin.actionRef;
}

function noMarketplaceActions(): MarketplacePluginActions {
  return {
    enable: false,
    disable: false,
    remove: false,
  };
}

function compareMarketplacePlugins(
  left: MarketplacePlugin,
  right: MarketplacePlugin,
) {
  if (left.source !== right.source) {
    return left.source === "builtin" ? -1 : 1;
  }
  return left.displayName.localeCompare(right.displayName);
}
