import { MenderApiError } from "@mender/sdk";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { sshRemoteCreateError } from "./ssh-remote-create-error";

beforeEach(() => {
  window.localStorage.clear();
  resetLocaleForTesting();
  setLocale("en");
});
afterEach(() => {
  window.localStorage.clear();
  resetLocaleForTesting();
});

describe("SSH Sandbox provision diagnostics", () => {
  it("maps each safe HTTP failure code to an actionable description", () => {
    const expected: Record<string, string> = {
      credential_unavailable: "OS credential store",
      credential_invalid: "not a supported private key",
      network_timeout: "timed out",
      network_refused: "refused the TCP connection",
      network_unreachable: "Could not reach",
      host_key_mismatch: "host identity verification failed",
      host_trust_unavailable: "trusted SSH host records",
      authentication_failed: "login was rejected",
      ssh_handshake_failed: "handshake failed",
      sftp_unavailable: "SFTP subsystem is unavailable",
      workspace_missing: "workspace directory does not exist",
      workspace_permission_denied: "cannot access the workspace",
      workspace_invalid: "not a real directory",
    };
    for (const [stage, fragment] of Object.entries(expected)) {
      const result = sshRemoteCreateError(new MenderApiError(
        503, "ssh_remote_" + stage, "secret-or-remote-message-MUST-NOT-RENDER",
      ));
      expect(result, stage).toContain(fragment);
      expect(result, stage).not.toContain("MUST-NOT-RENDER");
    }
  });

  it("recognizes safe Wails messages and rejected strings without echoing unknown details", () => {
    const error = new Error(
      "SSH remote connection failed: ssh_remote_failure:host_key_mismatch; raw: SECRET-KEY",
    );
    expect(sshRemoteCreateError(error)).toContain("host identity verification failed");
    expect(sshRemoteCreateError(error)).not.toContain("SECRET-KEY");
    expect(sshRemoteCreateError("ssh_remote_failure:authentication_failed")).toContain("login was rejected");
    expect(sshRemoteCreateError({ message: "ssh_remote_failure:workspace_missing" })).toContain("directory does not exist");
    expect(sshRemoteCreateError("ssh_remote_failure:unknown_injected_key")).toContain("exact stage is unavailable");
  });

  it("shows localized SSH-specific Chinese guidance rather than a generic unexpected error", () => {
    setLocale("zh-CN");
    const mismatch = sshRemoteCreateError(new Error("SSH remote connection failed: ssh_remote_failure:host_key_mismatch"));
    expect(mismatch).toContain("SSH 主机身份校验失败");
    expect(mismatch).toContain("登录密钥的公钥");
    expect(mismatch).not.toContain("发生意外错误");

    const missing = sshRemoteCreateError(new MenderApiError(503, "ssh_remote_workspace_missing", "secret"));
    expect(missing).toContain("远程工作区目录不存在");
    expect(missing).not.toContain("secret");

    const unknown = sshRemoteCreateError(new Error("sensitive remote user and /private/secret-path"));
    expect(unknown).toContain("SSH 沙盒创建失败");
    expect(unknown).not.toContain("/private/");
    expect(unknown).not.toContain("发生意外错误");

    expect(sshRemoteCreateError(new MenderApiError(403, "policy_denied", "server private data"))).toContain("安全策略");
    expect(sshRemoteCreateError(new MenderApiError(400, "validation_error", "private-key material"))).toContain("配置未通过校验");
  });
});
