import type {
  ManagementClient,
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
  install?: boolean;
  upgrade?: boolean;
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

export interface MarketplaceInstallAction {
  label: string;
  pluginId: string;
  version?: string;
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
  installAction?: MarketplaceInstallAction;
  actionRef?: string;
  issue?: MarketplacePluginIssue;
  pluginId?: string;
  usageCount?: number;
  updateAvailable?: string;
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

export function isVersionNewer(candidate: string, current: string): boolean {
  if (!candidate || !current || candidate === current) return false;
  const parse = (v: string) =>
    (v.replace(/^v/i, "").split(/[-+]/)[0] ?? "").split(".").map((n) => parseInt(n, 10) || 0);
  const candParts = parse(candidate);
  const currParts = parse(current);
  const len = Math.max(candParts.length, currParts.length);
  for (let i = 0; i < len; i++) {
    const c = candParts[i] ?? 0;
    const cur = currParts[i] ?? 0;
    if (c > cur) return true;
    if (c < cur) return false;
  }
  return false;
}

export function pluginCatalogueToMarketplace(entries:PluginCatalogueEntry[]):MarketplacePlugin[]{
  // Marketplace eligibility comes from the Host, not from capability claims
  // or a guessed route. Native installations remain for repair/removal.
  const registryVersions = new Map<string, string>();
  for (const entry of entries) {
    if (entry.sourceKind === "registry") {
      const currentLatest = registryVersions.get(entry.pluginId);
      if (!currentLatest || isVersionNewer(entry.packageVersion, currentLatest)) {
        registryVersions.set(entry.pluginId, entry.packageVersion);
      }
    }
  }

  return entries.filter(entry=>entry.marketplaceAction!=="none" || Boolean(entry.installationId) || entry.sourceKind==="registry")
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
    const installationId=entry.installationId;
    const isRegistryOffering = entry.sourceKind === "registry" && !installationId;
    const enabled=!isRegistryOffering && entry.enabled && entry.status==="enabled" && entry.issue===undefined;
    const action=entry.marketplaceAction;
    const actionKind=action==="create.provider"?"provider":action==="create.sandbox"?"sandbox":
      action==="create.agent"?"agent":action==="create.tunnel"?"tunnel":null;
    if(!isRegistryOffering && actionKind!==null && actionKind!==kind){
      throw new Error("Host Marketplace action and plugin extension kind disagree");
    }
    const params=new URLSearchParams({create:"1",pluginId:entry.pluginId});
    if(installationId)params.set("installationId",installationId);
    const pathname=action==="create.provider"?"/tools":
      action==="create.sandbox"?(installationId?"/sandboxes/new/external":"/sandboxes/new"):
      action==="create.agent"?"/agents/new":action==="create.tunnel"?"/tunnels":null;
    const createAction=!isRegistryOffering && enabled && pathname?{
      label:action==="create.provider"?"Add provider":action==="create.sandbox"?"Create sandbox":
        action==="create.agent"?"Configure agent":"Add tunnel",
      href:pathname+"?"+params.toString(),
    }:undefined;
    const latestRegistryVer = registryVersions.get(entry.pluginId);
    const updateAvailable = installationId && latestRegistryVer && isVersionNewer(latestRegistryVer, entry.packageVersion)
      ? latestRegistryVer
      : undefined;

    // An installation is managed by its exact ID, not by a creatable route.
    const manageableInstallation=!!installationId && ["provider","sandbox","tunnel"].includes(kind);
    const actions: MarketplacePluginActions = isRegistryOffering ? {
      enable: false,
      disable: false,
      remove: false,
      install: true,
    } : manageableInstallation ? {
      enable: entry.status === "disabled" && !entry.issue,
      disable: entry.status === "enabled" && entry.usageCount === 0,
      remove: entry.usageCount === 0,
      ...(updateAvailable ? { upgrade: entry.usageCount === 0 } : {}),
    } : noMarketplaceActions();

    const installAction: MarketplaceInstallAction | undefined = isRegistryOffering ? {
      label: "Install",
      pluginId: entry.pluginId,
      version: entry.packageVersion,
    } : undefined;

    return {
      key:entry.key,kind,kindLabel:labels[kind],
      displayName:entry.displayName||humanizePluginId(entry.pluginId),
      description:entry.description,
      source:entry.sourceKind,sourceLabel:entry.sourceKind==="builtin"?"Built-in":entry.sourceKind==="registry"?"Registry":"Local package",
      version:entry.packageVersion,
      status:entry.status==="unavailable"?"unavailable":entry.issue?"degraded":entry.status==="available"?"available":entry.status==="disabled"?"disabled":entry.status==="invalid"?"invalid":"enabled",
      enabled:entry.enabled,
      capabilities:entry.capabilities,
      actions,
      technicalDetails:[
        {label:"Plugin ID",value:entry.pluginId,mono:true},
        {label:"Extension ID",value:entry.extensionId,mono:true},
        {label:"Installation ID",value:installationId??(entry.sourceKind==="builtin"?"Built-in":"Uninstalled"),mono:!!installationId},
        ...(entry.digest?[{label:"Digest",value:entry.digest,mono:true}]:[]),
        ...(entry.installedAt?[{label:"Installed at",value:entry.installedAt,format:"dateTime" as const}]:[]),
        {label:"Used by resources",value:String(entry.usageCount ?? 0)},
        ...(entry.configVersion?[{label:"Config version",value:String(entry.configVersion)}]:[]),
        ...(entry.protocolVersion?[{label:"Protocol version",value:String(entry.protocolVersion)}]:[]),
      ],
      ...(createAction?{createAction}:{}),
      ...(installAction?{installAction}:{}),
      ...(installationId?{actionRef:installationId}:{}),
      ...(entry.issue?{issue:{code:entry.status==="unavailable"?"runtime_unavailable":"descriptor_unavailable",message:entry.issue}}:{}),
      pluginId: entry.pluginId,
      usageCount: entry.usageCount ?? 0,
      ...(updateAvailable ? { updateAvailable } : {}),
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
        href:"/tools?"+new URLSearchParams({create:"1",offeringId:entry.offeringId}).toString(),
      },
    };
  });
}

export async function loadMarketplacePlugins(client:ManagementClient){
  const [catalogue, integrations, updatesResult] = await Promise.all([
    client.listPluginCatalogue(),
    client.listIntegrationOfferings(),
    typeof client.listRegistryUpdates === "function"
      ? client.listRegistryUpdates().catch(() => ({ updates: [] }))
      : Promise.resolve({ updates: [] }),
  ]);
  const marketplacePlugins = [
    ...pluginCatalogueToMarketplace(catalogue.plugins),
    ...integrationOfferingsToMarketplace(integrations.integrations),
  ].toSorted(compareMarketplacePlugins);

  if (updatesResult?.updates?.length) {
    for (const update of updatesResult.updates) {
      const match = marketplacePlugins.find(
        (p) =>
          p.actionRef === update.installationId ||
          (p.pluginId === update.pluginId && p.actionRef !== undefined),
      );
      if (match) {
        match.updateAvailable = update.targetVersion;
      }
    }
  }

  return marketplacePlugins;
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
  client:ManagementClient,plugin:MarketplacePlugin,enabled:boolean,
){
  if(!(enabled?plugin.actions.enable:plugin.actions.disable)){
    throw new Error("Marketplace plugin does not support this state change");
  }
  const id=requireActionRef(plugin);
  if(enabled){await client.enablePluginInstallation(id);}
  else{await client.disablePluginInstallation(id);}
}

export async function removeMarketplacePlugin(
  client:ManagementClient,plugin:MarketplacePlugin,
){
  if(!plugin.actions.remove){throw new Error("Marketplace plugin does not support removal");}
  await client.deletePluginInstallation(requireActionRef(plugin));
}

export async function upgradeMarketplacePlugin(
  client: ManagementClient,
  plugin: MarketplacePlugin,
  targetVersion?: string,
): Promise<void> {
  const version = targetVersion ?? plugin.updateAvailable;
  if (!version) {
    throw new Error("Target version for upgrade is not specified");
  }
  if ((plugin.usageCount ?? 0) > 0) {
    throw new Error("Referenced resources block in-place upgrade");
  }
  const installationId = requireActionRef(plugin);
  await client.upgradeRegistryPlugin({
    installationId,
    version,
  });
}

export async function installMarketplacePlugin(
  client: ManagementClient,
  plugin: MarketplacePlugin,
  targetVersion?: string,
): Promise<void> {
  const pluginId = plugin.pluginId ?? plugin.installAction?.pluginId;
  if (!pluginId) {
    throw new Error("Plugin ID is required for installation");
  }
  const version =
    targetVersion ??
    plugin.installAction?.version ??
    (plugin.version !== "Reviewed preset" ? plugin.version : undefined);
  await client.installRegistryPlugin(
    version !== undefined ? { pluginId, version } : { pluginId },
  );
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
    const priority = (s: string) => (s === "builtin" ? 0 : s === "registry" ? 1 : s === "curated" ? 2 : 3);
    const diff = priority(left.source) - priority(right.source);
    if (diff !== 0) return diff;
    return left.source.localeCompare(right.source);
  }
  return left.displayName.localeCompare(right.displayName);
}
