import { t } from "../i18n/i18n";

/** Human-authored Mender identifiers: case and internal whitespace are presentation concerns.
 * This is deliberately NOT applied to external IDs, paths, endpoints, secrets or tool names. */
export function canonicalizeOwnedIdentifier(value: string): string {
  return value.trim().toLowerCase().replace(/\s+/gu, "-");
}

// Go catalog/domain.NewNamespace and profile/domain.NewSlug:
// 1..63 ASCII chars; initial a-z; trailing a-z0-9 unless single character.
export function isCanonicalOwnedSlug(value: string): boolean {
  return value.length <= 63 && /^(?:[a-z]|[a-z][a-z0-9-]*[a-z0-9])$/.test(value);
}

/** Match only known failure classes, never reflect arbitrary backend strings
 * that could contain provider credentials or sensitive configuration. */
export function ownedIdentifierCreateError(reason: unknown, kind: "profile" | "provider"): string {
  const message = typeof reason === "string" ? reason :
    reason instanceof Error ? reason.message :
    typeof reason === "object" && reason !== null && "message" in reason &&
      typeof reason.message === "string" ? reason.message : "";

  if (kind === "profile" && message.includes("profile: slug already exists")) {
    return t("This Profile slug is already in use.");
  }
  if (kind === "provider" && message.includes("catalog: provider namespace already exists")) {
    return t("This Provider namespace is already in use.");
  }
  if (kind === "profile" && message.includes("slug:")) {
    return t("Enter a valid Profile slug.");
  }
  if (kind === "provider" && message.includes("namespace:")) {
    return t("Enter a valid Provider namespace.");
  }
  return kind === "profile"
    ? t("Could not create Profile. Review the fields and try again.")
    : t("Could not create Provider. Review the configuration and try again.");
}
