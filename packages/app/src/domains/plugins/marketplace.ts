import type {
  MenderClient,
  PluginCatalogueEntry,
  ProviderPluginInstallation,
} from "@mender/sdk";

export type MarketplacePluginKind = string;
export type MarketplacePluginSource = string;
export type MarketplacePluginStatus =
  | "enabled"
  | "disabled"
  | "invalid"
  | "degraded"
  | "unavailable"
  | "planned";

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
  return entries.map((entry):MarketplacePlugin=>{
    const kind=entry.kind==="provider.tools"?"provider":
      entry.kind==="sandbox.backend"?"sandbox":"tunnel";
    const labels={provider:"Provider plugin",sandbox:"Sandbox backend",tunnel:"Tunnel transport"};
    const enabled=entry.enabled && entry.issue===undefined;
    const installationId=entry.installationId;
    const params=new URLSearchParams({create:"1",pluginId:entry.pluginId});
    if(installationId)params.set("installationId",installationId);
    const pathname=kind==="provider"?"/providers":
      kind==="sandbox"?(installationId?"/sandboxes/new/external":"/sandboxes/new"):"/tunnels";
    const createAction=enabled?{
      label:kind==="provider"?"Add provider":kind==="sandbox"?"Create sandbox":"Add tunnel",
      href:pathname+"?"+params.toString(),
    }:undefined;
    return {
      key:entry.key,kind,kindLabel:labels[kind],
      displayName:entry.displayName||humanizePluginId(entry.pluginId),
      description:entry.description,
      source:entry.sourceKind,sourceLabel:entry.sourceKind==="builtin"?"Built-in":
        entry.sourceKind==="planned"?"Roadmap":"Local package",
      version:entry.packageVersion||"Not released",
      status:entry.status==="unavailable"||entry.status==="planned"?
        entry.status:entry.issue?"degraded":entry.status,
      enabled:entry.enabled,
      capabilities:entry.capabilities,
      actions:installationId?{
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

export async function loadMarketplacePlugins(client:MenderClient){
  return pluginCatalogueToMarketplace((await client.listPluginCatalogue()).plugins);
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
