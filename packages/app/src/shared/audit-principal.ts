import type { Profile } from "@mender/sdk";

import { t } from "../i18n/i18n";

type AuditPrincipal = {
  principalKind: string;
  principalRef: string;
};

type KnownProfile = Pick<Profile, "profileId" | "displayName" | "slug">;

function referenceHint(reference: string): string {
  const tail = reference.split(":").at(-1) ?? reference;
  return tail.length > 10 ? "…" + tail.slice(-8) : tail || "—";
}

/**
 * Resolve the human-facing caller, not an authorization decision.
 * The original exact principal identifiers remain in the audit detail and
 * searchable list rows. Never guess a Profile name from an arbitrary ID.
 */
export function auditPrincipalLabel(
  principal: AuditPrincipal,
  profiles: ReadonlyArray<KnownProfile> = [],
): string {
  if (principal.principalKind === "service" && principal.principalRef.startsWith("profile:")) {
    const id = principal.principalRef.slice("profile:".length);
    const profile = profiles.find((item) => item.profileId === id);
    return t("Profile") + " · " +
      (profile?.displayName || profile?.slug || referenceHint(id));
  }
  const kind = principal.principalKind === "local_user" ? t("Local user") :
    principal.principalKind === "service" ? t("Service") : principal.principalKind;
  return (kind || "—") +
    (principal.principalRef ? " · " + referenceHint(principal.principalRef) : "");
}

export function auditPrincipalRaw(principal: AuditPrincipal): string {
  return principal.principalKind + (principal.principalRef ? ":" + principal.principalRef : "");
}
