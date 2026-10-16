import type { Provider } from "@mender/sdk";

import { t } from "../../i18n/i18n";

/** A service is named by the user or its exact resource owner, never by driver type. */
export function toolSourceLabel(provider: Provider | undefined): string {
  if (!provider) return t("Service unavailable");
  const name = provider.toolSourceName || provider.displayName || provider.namespace;
  if (provider.toolSourceKind === "sandbox") {
    return name + " · " +
      (provider.pluginId === "io.mender.provider.sandbox-jobs" ? t("Jobs") : t("Workspace"));
  }
  return name || t("Service unavailable");
}
