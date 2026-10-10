import { describe, expect, it } from "vitest";

import { canonicalizeOwnedIdentifier, isCanonicalOwnedSlug } from "./form-identifiers";
import { canonicalCredentialPart, validCredentialPart } from "../domains/credentials/credential-form";

describe("user-authored Mender identifiers", () => {
  it("normalizes case and whitespace without deleting other characters", () => {
    expect(canonicalizeOwnedIdentifier("  My  Profile  ")).toBe("my-profile");
    expect(canonicalizeOwnedIdentifier("OpenAI Agent")).toBe("openai-agent");
    expect(canonicalizeOwnedIdentifier("foo_Bar")).toBe("foo_bar");
    expect(canonicalizeOwnedIdentifier(" Foo ! ")).toBe("foo-!");
    expect(canonicalCredentialPart(" OpenAI Tunnel  Key ")).toBe("openai-tunnel-key");
  });

  it("matches Profile/Provider slug grammar, including end and length requirements", () => {
    for (const value of ["a", "alpha", "alpha-1", "a".repeat(63)]) {
      expect(isCanonicalOwnedSlug(value)).toBe(true);
    }
    for (const value of ["", "A", "1a", "alpha-", "-alpha", "alpha_B", "foo bar", "a".repeat(64), "árbol"]) {
      expect(isCanonicalOwnedSlug(value)).toBe(false);
    }
  });

  it("preserves the more permissive existing Credential identifier grammar", () => {
    expect(validCredentialPart("a")).toBe(true);
    expect(validCredentialPart("alpha-")).toBe(true);
    expect(validCredentialPart("a".repeat(63))).toBe(true);
    expect(validCredentialPart("alpha_beta")).toBe(false);
    expect(isCanonicalOwnedSlug("alpha-")).toBe(false);
  });
});
