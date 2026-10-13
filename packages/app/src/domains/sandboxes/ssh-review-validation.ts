/**
 * Client-side SSH Sandbox review hints. Cryptographic host key validation and
 * all trust decisions remain authoritative in sshremote.Template.Validate.
 */
export type SSHRemoteReviewField =
  | "name"
  | "host"
  | "port"
  | "user"
  | "root"
  | "hostPublicKey"
  | "credentialRef";

export type SSHRemoteReviewIssueCode =
  | "name_required"
  | "host_invalid"
  | "port_invalid"
  | "user_invalid"
  | "workspace_root_invalid"
  | "host_key_required"
  | "host_key_multiline"
  | "host_key_known_hosts_prefix"
  | "host_key_invalid"
  | "credential_required";

export interface SSHRemoteReviewIssue {
  field: SSHRemoteReviewField;
  code: SSHRemoteReviewIssueCode;
}

export interface SSHRemoteReviewDraft {
  name: string;
  host: string;
  port: number;
  user: string;
  root: string;
  hostPublicKey: string;
  credentialRef: string;
  credentialRefs: readonly string[];
}

const HOSTNAME_PATTERN = /^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$/;
const SSH_USER_PATTERN = /^[a-z_][a-z0-9_-]{0,62}$/;
const PUBLIC_KEY_ALGORITHM = /^(?:ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(?:256|384|521))$/;
const PUBLIC_KEY_LINE = /^(?:ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(?:256|384|521))[ \t]+[A-Za-z0-9+/]+={0,2}(?:[ \t]+[^\r\n]+)?$/;

function isIPv6Address(value: string): boolean {
  if (!value.includes(":") || !/^[0-9a-fA-F:.]+$/.test(value)) return false;
  try {
    // URL parsing is local and validates the IPv6 grammar; it does not resolve
    // DNS or contact the remote host.
    return new URL(`http://[${value}]/`).hostname !== "";
  } catch {
    return false;
  }
}

export function validateSSHRemoteReview(input: SSHRemoteReviewDraft): SSHRemoteReviewIssue[] {
  const issues: SSHRemoteReviewIssue[] = [];

  if (input.name.trim() === "" || new TextEncoder().encode(input.name.trim()).length > 160) {
    issues.push({ field: "name", code: "name_required" });
  }

  const host = input.host.trim();
  const isHostname = HOSTNAME_PATTERN.test(host) && !host.includes("..");
  // Permit IPv6 literals (including IPv4-mapped forms), mirroring the
  // backend's net.ParseIP without performing network requests.
  if (host.length > 253 || (!isHostname && !isIPv6Address(host))) {
    issues.push({ field: "host", code: "host_invalid" });
  }

  if (!Number.isInteger(input.port) || input.port < 1 || input.port > 65535) {
    issues.push({ field: "port", code: "port_invalid" });
  }

  if (!SSH_USER_PATTERN.test(input.user)) {
    issues.push({ field: "user", code: "user_invalid" });
  }

  const root = input.root.trim();
  const parts = root.split("/").slice(1);
  if (root.length < 2 || root.length > 1024 || !root.startsWith("/") ||
      parts.some((segment) => segment === "" || segment === "." || segment === "..") ||
      root.includes("\0") || root.includes("\r") || root.includes("\n")) {
    issues.push({ field: "root", code: "workspace_root_invalid" });
  }

  const key = input.hostPublicKey.trim();
  if (key === "") {
    issues.push({ field: "hostPublicKey", code: "host_key_required" });
  } else if (/\r|\n/.test(key)) {
    issues.push({ field: "hostPublicKey", code: "host_key_multiline" });
  } else if (key.length < 45 || key.length > 4096 || !PUBLIC_KEY_LINE.test(key)) {
    const tokens = key.split(/[ \t]+/);
    issues.push({
      field: "hostPublicKey",
      code: tokens.length >= 3 && PUBLIC_KEY_ALGORITHM.test(tokens[1] ?? "")
        ? "host_key_known_hosts_prefix"
        : "host_key_invalid",
    });
  }

  if (!input.credentialRefs.includes(input.credentialRef) || input.credentialRef === "") {
    issues.push({ field: "credentialRef", code: "credential_required" });
  }
  return issues;
}
