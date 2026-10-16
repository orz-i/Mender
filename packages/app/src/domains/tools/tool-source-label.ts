import type { Provider } from "@mender/sdk";

import { t } from "../../i18n/i18n";

/** Display-only projection. Exact owner is classified by the Host, not plugin metadata. */
export function toolSourceLabel(provider: Provider | undefined): string {
  if (!provider) return t("Unknown tool source");
  const name = provider.toolSourceName || provider.displayName || provider.namespace;
  switch (provider.toolSourceKind) {
    case "agent":
      return t("Agent") + " · " + name;
    case "sandbox":
      return t("Sandbox") + " · " + name +
        (provider.pluginId === "io.mender.provider.sandbox-jobs"
          ? " · " + t("Jobs Tool Suite")
          : " · " + t("Workspace Tool Suite"));
    case "unclaimed":
      return t("Unclaimed tool source");
    default:
      return name;
  }
}
