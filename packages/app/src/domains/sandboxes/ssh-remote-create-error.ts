import { MenderApiError, MenderClientInvariantError } from "@mender/sdk";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";

type SSHFailureCode =
  | "credential_unavailable"
  | "credential_invalid"
  | "network_timeout"
  | "network_refused"
  | "network_unreachable"
  | "host_key_mismatch"
  | "authentication_failed"
  | "ssh_handshake_failed"
  | "sftp_unavailable"
  | "workspace_missing"
  | "workspace_permission_denied"
  | "workspace_invalid";

function isSSHFailureCode(value: string): value is SSHFailureCode {
  switch (value) {
    case "credential_unavailable":
    case "credential_invalid":
    case "network_timeout":
    case "network_refused":
    case "network_unreachable":
    case "host_key_mismatch":
    case "authentication_failed":
    case "ssh_handshake_failed":
    case "sftp_unavailable":
    case "workspace_missing":
    case "workspace_permission_denied":
    case "workspace_invalid":
      return true;
    default:
      return false;
  }
}

function sshFailureCode(reason: unknown): SSHFailureCode | null {
  if (reason instanceof MenderApiError) {
    const suffix = reason.code.startsWith("ssh_remote_")
      ? reason.code.slice("ssh_remote_".length) : "";
    return isSSHFailureCode(suffix) ? suffix : null;
  }

  // Wails returns error strings (or Error objects) rather than structured API
  // codes. Only the first-party safe diagnostic marker is recognized.
  const detail = typeof reason === "string" ? reason :
    reason instanceof Error ? reason.message :
    typeof reason === "object" && reason !== null &&
      "message" in reason && typeof reason.message === "string"
      ? reason.message : "";

  const code = /(?:^|\b)ssh_remote_failure:([a-z_]+)(?:\b|$)/u.exec(detail)?.[1] ?? "";
  return isSSHFailureCode(code) ? code : null;
}

/**
 * SSH-specific failure feedback: never reflect an arbitrary Wails error or
 * remote server response. API/Wails may contain configuration details, so
 * only known and explicitly safe stage codes reach the screen.
 */
export function sshRemoteCreateError(reason: unknown): string {
  switch (sshFailureCode(reason)) {
    case "credential_unavailable":
      return t("Cannot access the selected SSH private key from the OS credential store. Check that the credential exists and that Keychain access is allowed.");
    case "credential_invalid":
      return t("The selected SSH credential is not a supported private key. Reimport or rotate the SSH credential.");
    case "network_timeout":
      return t("SSH connection timed out. Check the host address, port, network, and firewall.");
    case "network_refused":
      return t("The SSH host refused the TCP connection. Check that SSH is running and listening on the configured port.");
    case "network_unreachable":
      return t("Could not reach the SSH host. Check its address, network route, and firewall.");
    case "host_key_mismatch":
      return t("SSH host identity verification failed. Compare the pinned host public key with the server's verified SSH host key. Do not use your login key's public key or bypass verification.");
    case "authentication_failed":
      return t("SSH public-key login was rejected. Verify the selected private key, the remote account's authorized_keys, and whether the server permits public-key login for this account.");
    case "ssh_handshake_failed":
      return t("The SSH handshake failed. Check the server's SSH host-key algorithms and connectivity. Do not disable host-key verification.");
    case "sftp_unavailable":
      return t("SSH authentication succeeded, but the server's SFTP subsystem is unavailable. Enable SFTP on the remote SSH server.");
    case "workspace_missing":
      return t("SSH and SFTP connected, but the remote workspace directory does not exist. Create it on the server or select an existing directory.");
    case "workspace_permission_denied":
      return t("SSH and SFTP connected, but the remote account cannot access the workspace directory. Check its directory permissions.");
    case "workspace_invalid":
      return t("SSH and SFTP connected, but the workspace path is not a real directory or contains a symbolic link. Choose an existing directory without symlinked parents.");
    default:
      if (reason instanceof MenderClientInvariantError) {
        return errorMessage(reason);
      }
      if (reason instanceof MenderApiError && reason.code === "policy_denied") {
        return t("SSH Sandbox provisioning was denied by the local security policy. Check Sandbox Policy and your access permissions.");
      }
      if (reason instanceof MenderApiError && reason.code === "validation_error") {
        return t("SSH Sandbox configuration was rejected. Review the host key, account, workspace path, and credential.");
      }
      return t("SSH Sandbox creation failed. The exact stage is unavailable; check SSH connectivity, host identity, login authorization, SFTP, and the remote workspace directory.");
  }
}
