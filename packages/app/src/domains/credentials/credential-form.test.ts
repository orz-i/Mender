import { describe, expect, it } from "vitest";

import {
  availableCredentialName,
  credentialReferencePreview,
  suggestedCredentialName,
  validCredentialPart,
} from "./credential-form";

describe("human-readable Credential name to stable technical identity", () => {
  it("preserves the existing behavior for ordinary ASCII titles", () => {
    expect(suggestedCredentialName("GitHub Work")).toBe("github-work");
    expect(suggestedCredentialName("  openai   tunnel ")).toBe("openai-tunnel");
    expect(suggestedCredentialName("")).toBe("");
  });

  it.each([
    "项目密钥",
    "日本語のキー",
    "🔑",
    "Release 🔑 钥匙",
    "Café",
    "123#unusual",
    "A".repeat(155) + "密钥",
  ])("derives an ASCII identifier from %s without requiring Latin input", (displayName) => {
    const name = suggestedCredentialName(displayName);
    expect(name.length).toBeGreaterThan(0);
    expect(name.length).toBeLessThanOrEqual(63);
    expect(validCredentialPart(name)).toBe(true);
    expect(credentialReferencePreview("personal", name)).toBe(`credential://personal/${name}`);
    expect(suggestedCredentialName(displayName)).toBe(name);
  });

  it("normalizes canonically equivalent Unicode inputs", () => {
    expect(suggestedCredentialName("Café")).toBe(suggestedCredentialName("Cafe\u0301"));
    expect(suggestedCredentialName("ＡＢ")).toBe("ab");
    expect(suggestedCredentialName("项目密钥")).not.toBe(suggestedCredentialName("项目钥匙"));
  });

  it("avoids a known duplicate in the same namespace without altering other namespaces", () => {
    const base = suggestedCredentialName("项目密钥");
    const inventory = [
      `credential://personal/${base}`,
      `credential://personal/${base}-2`,
      "credential://ssh/project-github",
    ];
    expect(availableCredentialName("项目密钥", "personal", inventory)).toBe(`${base}-3`);
    expect(availableCredentialName("项目密钥", "ssh", inventory)).toBe(base);
    expect(availableCredentialName("Project GitHub", "ssh", inventory)).toBe("project-github-2");
  });

  it("bounds suffixes even when the original Latin identifier uses all 63 bytes", () => {
    const longName = "a".repeat(63);
    const candidate = availableCredentialName(longName, "personal", [
      `credential://personal/${longName}`,
    ]);
    expect(candidate).toHaveLength(63);
    expect(candidate.endsWith("-2")).toBe(true);
    expect(validCredentialPart(candidate)).toBe(true);
  });
});
