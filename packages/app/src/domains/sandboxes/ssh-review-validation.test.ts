import { describe, expect, it } from "vitest";
import { validateSSHRemoteReview, type SSHRemoteReviewDraft } from "./ssh-review-validation";

const pin = "ssh-ed25519 " + "A".repeat(68);
const valid: SSHRemoteReviewDraft = {
  name: "Rocky",
  host: "203.0.113.25",
  port: 22,
  user: "agent",
  root: "/data/workspace",
  hostPublicKey: pin,
  credentialRef: "credential://ssh/rocky",
  credentialRefs: ["credential://ssh/rocky"],
};

function invalidCodes(patch: Partial<SSHRemoteReviewDraft>): string[] {
  return validateSSHRemoteReview({ ...valid, ...patch }).map((issue) => issue.code);
}

describe("SSH Sandbox review validation", () => {
  it("accepts canonical SSH configuration with typed host key and terminal newline", () => {
    expect(invalidCodes({})).toEqual([]);
    expect(invalidCodes({ hostPublicKey: pin + "\n" })).toEqual([]);
    expect(invalidCodes({ hostPublicKey: "ssh-rsa " + "A".repeat(68) })).toEqual([]);
    expect(invalidCodes({ hostPublicKey: "ecdsa-sha2-nistp256 " + "A".repeat(68) })).toEqual([]);
    expect(invalidCodes({ host: "2001:db8::1" })).toEqual([]);
  });

  it("permits root login but still rejects a malformed pinned host key without echoing its contents", () => {
    const codes = invalidCodes({
      user: "root",
      hostPublicKey: "AAAAB3NzaC1yc2EAAAADAQABAAAB",
    });
    expect(codes).not.toContain("user_invalid");
    expect(codes).toContain("host_key_invalid");
    expect(JSON.stringify(validateSSHRemoteReview({
      ...valid,
      user: "root",
      hostPublicKey: "PRIVATE-TEST-PAYLOAD-NEVER-RENDER",
    }))).not.toContain("PRIVATE-TEST-PAYLOAD");
  });

  it("rejects missing, multi-line, malformed and known_hosts-prefixed pins", () => {
    expect(invalidCodes({ hostPublicKey: "" })).toContain("host_key_required");
    expect(invalidCodes({ hostPublicKey: pin + "\n" + pin })).toContain("host_key_multiline");
    expect(invalidCodes({ hostPublicKey: "203.0.113.25 " + pin })).toContain("host_key_known_hosts_prefix");
    expect(invalidCodes({ hostPublicKey: "ssh-dss " + "A".repeat(68) })).toContain("host_key_invalid");
    expect(invalidCodes({ hostPublicKey: "ssh-ed25519 abc&def" })).toContain("host_key_invalid");
    expect(invalidCodes({ hostPublicKey: "ssh-ed25519 " + "A".repeat(4096) })).toContain("host_key_invalid");
  });

  it("rejects invalid account, canonical root, port, host, and unknown credential", () => {
    expect(invalidCodes({ user: "Agent" })).toContain("user_invalid");
    expect(invalidCodes({ user: "root" })).toEqual([]);
    expect(invalidCodes({ root: "/" })).toContain("workspace_root_invalid");
    expect(invalidCodes({ root: "/data//workspace" })).toContain("workspace_root_invalid");
    expect(invalidCodes({ root: "/data/../etc" })).toContain("workspace_root_invalid");
    expect(invalidCodes({ root: "/data/workspace/" })).toContain("workspace_root_invalid");
    expect(invalidCodes({ port: 0 })).toContain("port_invalid");
    expect(invalidCodes({ port: 65536 })).toContain("port_invalid");
    expect(invalidCodes({ host: "example..com" })).toContain("host_invalid");
    expect(invalidCodes({ host: ":::1" })).toContain("host_invalid");
    expect(invalidCodes({ name: "" })).toContain("name_required");
    expect(invalidCodes({ credentialRef: "credential://other/key" })).toContain("credential_required");
  });
});
