import { describe, expect, it } from "vitest";
import { validateSSHRemoteReview, type SSHRemoteReviewDraft } from "./ssh-review-validation";

const valid: SSHRemoteReviewDraft = {
  name: "Rocky",
  host: "203.0.113.25",
  port: 22,
  user: "agent",
  root: "/data/workspace",
  credentialRef: "credential://ssh/rocky",
  credentialRefs: ["credential://ssh/rocky"],
};

function invalidCodes(patch: Partial<SSHRemoteReviewDraft>): string[] {
  return validateSSHRemoteReview({ ...valid, ...patch }).map((issue) => issue.code);
}

describe("SSH Sandbox review validation", () => {
  it("accepts SSH connection configuration without requesting a host public key", () => {
    expect(invalidCodes({})).toEqual([]);
    expect(invalidCodes({host: "2001:db8::1"})).toEqual([]);
    expect(invalidCodes({user: "root"})).toEqual([]);
  });

  it("rejects invalid account, canonical root, port, host, and missing credential", () => {
    expect(invalidCodes({ user: "Agent" })).toContain("user_invalid");
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
