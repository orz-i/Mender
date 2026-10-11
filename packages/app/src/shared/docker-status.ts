import { t } from "../i18n/i18n";

// Engine readiness is host-observed, not a promise of perpetual connectivity.
// Keep the reason codes stable and avoid leaking raw daemon diagnostics.
export function dockerStatusText(status: string): string {
  switch (status) {
    case "Docker Engine not detected":
      return t("Docker Engine not detected");
    case "Docker Engine is not configured":
      return t("Docker Engine is not configured");
    case "Docker Engine unavailable or insecure":
      return t("Docker Engine unavailable or insecure");
    case "Docker Engine disconnected or insecure; retrying":
      return t("Docker Engine disconnected or insecure; retrying");
    case "Docker Engine identity changed; restart Mender to review":
      return t("Docker Engine identity changed; restart Mender to review");
    default:
      return status;
  }
}
