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
  | "credentialRef";

export type SSHRemoteReviewIssueCode =
  | "name_required"
  | "host_invalid"
  | "port_invalid"
  | "user_invalid"
  | "workspace_root_invalid"
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
  credentialRef: string;
  credentialRefs: readonly string[];
}

const HOSTNAME_PATTERN = /^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$/;
const SSH_USER_PATTERN = /^[a-z_][a-z0-9_-]{0,62}$/;

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

  if (!input.credentialRefs.includes(input.credentialRef) || input.credentialRef === "") {
    issues.push({ field: "credentialRef", code: "credential_required" });
  }
  return issues;
}
