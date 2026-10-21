import { t } from "../../i18n/i18n";
import { canonicalizeOwnedIdentifier } from "../../shared/form-identifiers";

// Keep these checks aligned with credential/domain/reference.go and credential.go.
export const CREDENTIAL_PART_PATTERN = /^[a-z][a-z0-9-]{0,62}$/;
export const ENVIRONMENT_VARIABLE_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;
export const MAX_CREDENTIAL_DISPLAY_NAME_BYTES = 160;
export const MAX_NATIVE_SECRET_BYTES = 64 * 1024;

export function canonicalCredentialPart(value: string): string {
  return canonicalizeOwnedIdentifier(value);
}

export function validCredentialPart(value: string): boolean {
  return CREDENTIAL_PART_PATTERN.test(value);
}

/** A readable title is not necessarily a Go-compatible Credential identity.
 * Preserve valid Latin slugs; otherwise append a stable, non-secret 64-bit
 * fingerprint of the normalized title. This is an identifier, not a security
 * digest: the server remains authoritative for duplicate references. */
export function suggestedCredentialName(displayName: string): string {
  const normalized = displayName.trim().normalize("NFKC");
  if (!normalized) return "";
  const canonical = canonicalCredentialPart(normalized);
  if (validCredentialPart(canonical)) return canonical;

  const latinStem = normalized.normalize("NFKD")
    .replace(/\p{M}/gu, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  const stem = /^[a-z]/.test(latinStem) ? latinStem : "credential";
  let fingerprint = 0xcbf29ce484222325n;
  for (const byte of new TextEncoder().encode(normalized)) {
    fingerprint = BigInt.asUintN(
      64, (fingerprint ^ BigInt(byte)) * 0x100000001b3n,
    );
  }
  const suffix = "-u-" + fingerprint.toString(16).padStart(16, "0");
  return stem.slice(0, 63 - suffix.length).replace(/-+$/g, "") + suffix;
}

/** Keep existing references intact by choosing the next unused name from the
 * currently loaded inventory. A concurrent create is still rejected by Go. */
export function availableCredentialName(
  displayName: string,
  namespace: string,
  existingReferences: readonly string[],
): string {
  const base = suggestedCredentialName(displayName);
  if (!base) return "";
  const taken = new Set(existingReferences);
  const reference = (name: string) => credentialReferencePreview(
    canonicalCredentialPart(namespace), name,
  );
  const requestedRef = reference(base);
  if (requestedRef === null || !taken.has(requestedRef)) return base;
  for (let ordinal = 2; ordinal <= existingReferences.length + 2; ordinal++) {
    const suffix = "-" + ordinal;
    const candidate = base.slice(0, 63 - suffix.length)
      .replace(/-+$/g, "") + suffix;
    const candidateRef = reference(candidate);
    if (candidateRef === null || !taken.has(candidateRef)) return candidate;
  }
  return base; // Backend uniqueness check is authoritative if inventory is stale.
}

export function credentialReferencePreview(namespace: string, name: string): string | null {
  if (!validCredentialPart(namespace) || !validCredentialPart(name)) return null;
  return `credential://${namespace}/${name}`;
}

export function credentialCreateError(reason: unknown): string {
  // Wails can reject with an Error or a string. Classify only known, safe
  // operational errors; never render arbitrary backend errors near secret inputs.
  const detail =
    typeof reason === "string" ? reason :
    reason instanceof Error ? reason.message :
    typeof reason === "object" && reason !== null && "message" in reason &&
      typeof reason.message === "string" ? reason.message : "";

  if (detail.includes("SSH private key credentials must use")) {
    return t("SSH keys must use the native OS credential store, not environment variables.");
  }
  if (detail.includes("ssh private key: passphrase too long")) {
    return t("SSH key passphrase is too long.");
  }
  if (detail.includes("ssh private key: passphrase required")) {
    return t("This SSH private key is encrypted. Enter its passphrase.");
  }
  if (detail.includes("ssh private key: incorrect passphrase")) {
    return t("Incorrect SSH key passphrase or unreadable encrypted key.");
  }
  if (detail.includes("ssh private key: RSA key must be at least 2048 bits")) {
    return t("RSA private keys must be at least 2048 bits. Use a newer key.");
  }
  if (detail.includes("ssh private key: invalid key format")) {
    return t("This is not a supported PEM or OpenSSH private key.");
  }
  if (detail.includes("ssh private key:")) {
    return t("Use an Ed25519, ECDSA or RSA 2048+ private key stored in the OS keychain.");
  }
  if (detail.includes("credential: reference already exists")) {
    return t("A credential with this reference already exists.");
  }
  if (detail.includes("credential: secret source unavailable") ||
      detail.includes("native keyring write failed")) {
    return t("The OS credential store is unavailable. Check keychain access and try again.");
  }
  if (detail.includes("credential reference: namespace and name must match")) {
    return t("Use valid namespace and name identifiers before creating a credential.");
  }
  if (detail.includes("credential environment_variable: invalid variable name")) {
    return t("Enter a valid environment variable name.");
  }
  return t("Credential creation failed. Check the fields and try again.");
}
