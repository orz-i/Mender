import type {
  MenderClient,
  ProviderPlugin,
  ProviderPluginInstallation,
  ProviderPluginIssue,
} from "@mender/sdk";

export type MarketplacePluginKind = string;
export type MarketplacePluginSource = string;
export type MarketplacePluginStatus =
  | "enabled"
  | "disabled"
  | "invalid"
  | "degraded";

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

export function providerPluginInstaller(
  install: () => Promise<ProviderPluginInstallation | null>,
): MarketplaceInstaller {
  return {
    key: "provider-local-package",
    label: "Install Provider plugin",
    pendingLabel: "Installing…",
    async install() {
      return (await install()) !== null;
    },
  };
}

interface MarketplaceAdapter {
  kind: MarketplacePluginKind;
  load: (client: MenderClient) => Promise<MarketplacePlugin[]>;
  setEnabled?: (
    client: MenderClient,
    plugin: MarketplacePlugin,
    enabled: boolean,
  ) => Promise<void>;
  remove?: (
    client: MenderClient,
    plugin: MarketplacePlugin,
  ) => Promise<void>;
}

const providerMarketplaceAdapter: MarketplaceAdapter = {
  kind: "provider",

  async load(client) {
    const [catalog, installations] = await Promise.all([
      client.listProviderPlugins(),
      client.listProviderPluginInstallations(),
    ]);
    return providerPluginsToMarketplace(
      catalog.plugins,
      installations.installations,
      catalog.issues,
    );
  },

  async setEnabled(client, plugin, enabled) {
    const installationId = requireActionRef(plugin);
    if (enabled) {
      await client.enableProviderPluginInstallation(installationId);
      return;
    }
    await client.disableProviderPluginInstallation(installationId);
  },

  async remove(client, plugin) {
    await client.deleteProviderPluginInstallation(requireActionRef(plugin));
  },
};

const marketplaceAdapters: readonly MarketplaceAdapter[] = [
  providerMarketplaceAdapter,
];

export async function loadMarketplacePlugins(client: MenderClient) {
  const groups = await Promise.all(
    marketplaceAdapters.map((adapter) => adapter.load(client)),
  );
  return groups.flat().toSorted(compareMarketplacePlugins);
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
  client: MenderClient,
  plugin: MarketplacePlugin,
  enabled: boolean,
) {
  const adapter = marketplaceAdapterFor(plugin);
  if (
    adapter.setEnabled === undefined ||
    (enabled ? !plugin.actions.enable : !plugin.actions.disable)
  ) {
    throw new Error("Marketplace plugin does not support this state change");
  }
  await adapter.setEnabled(client, plugin, enabled);
}

export async function removeMarketplacePlugin(
  client: MenderClient,
  plugin: MarketplacePlugin,
) {
  const adapter = marketplaceAdapterFor(plugin);
  if (adapter.remove === undefined || !plugin.actions.remove) {
    throw new Error("Marketplace plugin does not support removal");
  }
  await adapter.remove(client, plugin);
}

export function providerPluginsToMarketplace(
  plugins: ProviderPlugin[],
  installations: ProviderPluginInstallation[],
  issues: ProviderPluginIssue[],
): MarketplacePlugin[] {
  const descriptorByInstallation = new Map(
    plugins
      .filter(
        (
          plugin,
        ): plugin is ProviderPlugin & {
          installationId: string;
        } => plugin.installationId !== undefined,
      )
      .map((plugin) => [plugin.installationId, plugin] as const),
  );
  const issueByInstallation = new Map(
    issues.map((issue) => [issue.installationId, issue] as const),
  );

  const builtins = plugins
    .filter((plugin) => plugin.sourceKind === "builtin")
    .map((plugin) =>
      marketplacePluginFromDescriptor(plugin, undefined, undefined),
    );

  const external = installations.map((installation) => {
    const descriptor = descriptorByInstallation.get(
      installation.installationId,
    );
    const issue = issueByInstallation.get(installation.installationId);
    return marketplacePluginFromInstallation(
      installation,
      descriptor,
      issue,
    );
  });

  const knownExternalInstallations = new Set(
    installations.map((installation) => installation.installationId),
  );
  const descriptorOnlyExternal = plugins
    .filter(
      (plugin) =>
        plugin.sourceKind === "external" &&
        plugin.installationId !== undefined &&
        !knownExternalInstallations.has(plugin.installationId),
    )
    .map((plugin) =>
      marketplacePluginFromDescriptor(
        plugin,
        plugin.installationId === undefined
          ? undefined
          : issueByInstallation.get(plugin.installationId),
        "local",
      ),
    );

  return [...builtins, ...external, ...descriptorOnlyExternal].toSorted(
    compareMarketplacePlugins,
  );
}

function marketplacePluginFromDescriptor(
  plugin: ProviderPlugin,
  issue: ProviderPluginIssue | undefined,
  sourceOverride?: MarketplacePluginSource,
): MarketplacePlugin {
  const builtIn = plugin.sourceKind === "builtin";
  const source =
    sourceOverride ??
    (builtIn ? "builtin" : "local");
  const createAction = providerCreateAction({
    pluginId: plugin.pluginId,
    installationId: plugin.installationId,
    enabled: plugin.enabled && issue === undefined,
  });

  return {
    key: plugin.installationId
      ? plugin.pluginId + "@" + plugin.installationId
      : plugin.pluginId + "@builtin",
    kind: "provider",
    kindLabel: "Provider plugin",
    displayName: plugin.displayName || humanizePluginId(plugin.pluginId),
    description: plugin.description,
    source,
    sourceLabel: source === "builtin" ? "Built-in" : "Local package",
    version: plugin.packageVersion || plugin.version,
    status:
      issue === undefined
        ? plugin.enabled
          ? "enabled"
          : "disabled"
        : "degraded",
    enabled: plugin.enabled,
    capabilities: plugin.capabilities,
    actions: noMarketplaceActions(),
    technicalDetails: providerTechnicalDetails({
      pluginId: plugin.pluginId,
      installationId: plugin.installationId,
      digest: undefined,
      installedAt: undefined,
      configVersion: plugin.configVersion,
      protocolVersion: plugin.protocolVersion,
    }),
    ...(createAction === undefined ? {} : { createAction }),
    ...(issue === undefined
      ? {}
      : { issue: marketplaceIssue(issue) }),
  };
}

function marketplacePluginFromInstallation(
  installation: ProviderPluginInstallation,
  descriptor: ProviderPlugin | undefined,
  issue: ProviderPluginIssue | undefined,
): MarketplacePlugin {
  const status: MarketplacePluginStatus =
    issue !== undefined
      ? "degraded"
      : installation.status === "invalid"
        ? "invalid"
        : installation.status;
  const enabled = installation.status === "enabled";
  const createAction = providerCreateAction({
    pluginId: installation.pluginId,
    installationId: installation.installationId,
    enabled: enabled && descriptor !== undefined && issue === undefined,
  });

  return {
    key: installation.pluginId + "@" + installation.installationId,
    kind: "provider",
    kindLabel: "Provider plugin",
    displayName:
      installation.displayName ??
      descriptor?.displayName ??
      humanizePluginId(installation.pluginId),
    description:
      installation.description ??
      descriptor?.description ??
      "",
    source: "local",
    sourceLabel: "Local package",
    version: installation.packageVersion,
    status,
    enabled,
    capabilities:
      installation.capabilities ??
      descriptor?.capabilities ??
      [],
    actions: {
      enable:
        !enabled &&
        installation.status !== "invalid" &&
        issue === undefined,
      disable: enabled,
      remove: true,
    },
    technicalDetails: providerTechnicalDetails({
      pluginId: installation.pluginId,
      installationId: installation.installationId,
      digest: installation.digest,
      installedAt: installation.installedAt,
      configVersion:
        installation.configVersion ?? descriptor?.configVersion,
      protocolVersion:
        installation.protocolVersion ?? descriptor?.protocolVersion,
    }),
    ...(createAction === undefined ? {} : { createAction }),
    actionRef: installation.installationId,
    ...(issue === undefined
      ? {}
      : { issue: marketplaceIssue(issue) }),
  };
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

function marketplaceAdapterFor(plugin: MarketplacePlugin) {
  const adapter = marketplaceAdapters.find(
    (candidate) => candidate.kind === plugin.kind,
  );
  if (adapter === undefined) {
    throw new Error("Marketplace plugin type has no registered adapter");
  }
  return adapter;
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

function providerCreateAction({
  pluginId,
  installationId,
  enabled,
}: {
  pluginId: string;
  installationId: string | undefined;
  enabled: boolean;
}): MarketplaceCreateAction | undefined {
  if (!enabled) {
    return undefined;
  }

  const search = new URLSearchParams({
    create: "1",
    pluginId,
  });
  if (installationId !== undefined) {
    search.set("installationId", installationId);
  }
  return {
    label: "Add provider",
    href: `/providers?${search.toString()}`,
  };
}

function marketplaceIssue(
  issue: ProviderPluginIssue,
): MarketplacePluginIssue {
  return {
    code: issue.code,
    message: issue.message,
  };
}

function providerTechnicalDetails({
  pluginId,
  installationId,
  digest,
  installedAt,
  configVersion,
  protocolVersion,
}: {
  pluginId: string;
  installationId: string | undefined;
  digest: string | undefined;
  installedAt: string | undefined;
  configVersion: number | undefined;
  protocolVersion: number | undefined;
}): MarketplaceTechnicalDetail[] {
  return [
    {
      label: "Plugin ID",
      value: pluginId,
      mono: true,
    },
    {
      label: "Installation ID",
      value: installationId ?? "Built-in",
      mono: installationId !== undefined,
    },
    {
      label: "Digest",
      value: digest ?? "Built-in",
      mono: digest !== undefined,
    },
    {
      label: "Installed",
      value: installedAt ?? "Built-in",
      format: installedAt === undefined ? "text" : "dateTime",
    },
    {
      label: "Config version",
      value:
        configVersion === undefined
          ? "Unavailable"
          : String(configVersion),
    },
    {
      label: "Protocol version",
      value:
        protocolVersion === undefined
          ? "Unavailable"
          : String(protocolVersion),
    },
  ];
}
