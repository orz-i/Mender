import type { Provider, ProviderPlugin } from "@mender/sdk";

import { t } from "../../../i18n/i18n";
import { genericConfigDefaults } from "../../../shared/plugin-configuration";
import {
  MCP_PLUGIN_ID,
  MODEL_TOOLS_PLUGIN_ID,
  OPENAPI_PLUGIN_ID,
  WORKSPACE_PLUGIN_ID,
  type ProviderFormState,
  providerRenderer,
} from "./provider-form-model";

export function providerPluginSelectionKey(plugin: ProviderPlugin) {
  return plugin.installationId
    ? plugin.pluginId + "@" + plugin.installationId
    : plugin.pluginId + "@builtin";
}

export function providerFormSelectionKey(form: ProviderFormState) {
  return form.installationId
    ? form.pluginId + "@" + form.installationId
    : form.pluginId + "@builtin";
}

function providerPluginMatchesForm(
  plugin: ProviderPlugin,
  form: ProviderFormState,
) {
  return (
    plugin.pluginId === form.pluginId &&
    (plugin.installationId ?? "") === form.installationId
  );
}

export function providerPluginForForm(
  plugins: ProviderPlugin[],
  form: ProviderFormState,
) {
  return plugins.find((plugin) => providerPluginMatchesForm(plugin, form));
}

export function providerPluginChoiceLabel(plugin: ProviderPlugin) {
  return plugin.installationId
    ? plugin.displayName + " · " + plugin.packageVersion
    : plugin.displayName;
}

export function providerPluginForProvider(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  return plugins.find(
    (plugin) =>
      plugin.pluginId === provider.pluginId &&
      (plugin.installationId ?? "") === (provider.installationId ?? ""),
  );
}

export function providerPluginLabelForProvider(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  const plugin = providerPluginForProvider(plugins, provider);
  if (plugin !== undefined) {
    return providerPluginChoiceLabel(plugin);
  }
  if (provider.pluginDisplayName) {
    return provider.pluginDisplayName;
  }
  switch (provider.pluginId) {
    case MCP_PLUGIN_ID:
      return t("MCP");
    case MODEL_TOOLS_PLUGIN_ID:
      return "Model Tools";
    case OPENAPI_PLUGIN_ID:
      return "OpenAPI";
    case WORKSPACE_PLUGIN_ID:
      return t("Workspace");
    default:
      return t("Unconfigured");
  }
}

export function providerIsBuiltIn(provider: Provider, pluginId: string) {
  return provider.installationId === undefined && provider.pluginId === pluginId;
}

export function providerRebindCandidates(
  plugins: ProviderPlugin[],
  provider: Provider,
) {
  if (!provider.installationId || !provider.pluginId) {
    return [];
  }
  return plugins.filter(
    (plugin) =>
      plugin.sourceKind === "external" &&
      plugin.pluginId === provider.pluginId &&
      plugin.installationId !== undefined &&
      plugin.installationId !== provider.installationId,
  );
}

export function providerSupportsFirstPartyAuthentication(provider: Provider) {
  return (
    provider.installationId === undefined &&
    (provider.pluginId === MCP_PLUGIN_ID ||
      provider.pluginId === OPENAPI_PLUGIN_ID ||
      provider.pluginId === MODEL_TOOLS_PLUGIN_ID)
  );
}

export function formForProviderPlugin(
  form: ProviderFormState,
  plugin: ProviderPlugin,
): ProviderFormState {
  const installationId = plugin.installationId ?? "";
  const next = {
    ...form,
    pluginId: plugin.pluginId,
    installationId,
  };
  return {
    ...next,
    genericConfig:
      providerRenderer(next) === "generic"
        ? genericConfigDefaults(plugin.configSchema)
        : {},
  };
}
