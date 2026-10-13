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
