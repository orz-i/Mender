import type { Sandbox } from "@mender/sdk";
import { t } from "../../i18n/i18n";

export function sandboxBackend(item: Sandbox): string {
  return item.backend === "local_workspace" ? t("Local workspace") :
    item.backend === "ssh_remote" ? t("SSH Remote") : item.backend;
}

export function isolationLabel(value: string): string {
  if (value === "host") return t("Host (not isolated)");
  if (value === "restricted_host") return t("Restricted host");
  return value;
}

export function sandboxStateLabel(state: Sandbox["state"]): string {
  switch (state) {
    case "ready": return t("Ready");
    case "stopped": return t("Stopped");
    case "destroyed": return t("Destroyed");
    default: return state;
  }
}
