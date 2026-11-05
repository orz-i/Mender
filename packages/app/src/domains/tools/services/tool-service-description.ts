import type { Provider } from "@mender/sdk";
import { t } from "../../../i18n/i18n";

/** Translate only host-generated, owner-verified Tool service summaries.
 * User and third-party descriptions are immutable display data.
 */
export function toolServiceDescription(provider: Provider): string {
  const name = provider.toolSourceName;
  if (!name) return provider.description;

  if (
    provider.toolSourceKind === "agent" &&
    provider.pluginId === "io.mender.provider.agent-delegation" &&
    provider.description === "Managed Agent Delegation tools for " + name
  ) {
    return t("Managed Agent Delegation tools for {{name}}", { name });
  }
  if (
    provider.toolSourceKind === "sandbox" &&
    provider.pluginId === "io.mender.provider.workspace" &&
    provider.description === "Managed Workspace Tools for " + name
  ) {
    return t("Managed Workspace Tools for {{name}}", { name });
  }
  if (
    provider.toolSourceKind === "sandbox" &&
    provider.pluginId === "io.mender.provider.sandbox-jobs" &&
    provider.description === "Owner-managed SSH Durable Jobs for " + name
  ) {
    return t("Owner-managed SSH Durable Jobs for {{name}}", { name });
  }
  return provider.description;
}
