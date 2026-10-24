import type { Provider } from "@mender/sdk";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { toolServiceDescription } from "./tool-service-description";

const base: Provider = {
  providerId: "prov_fixture", namespace: "fixture", displayName: "Fixture",
  description: "", transport: "http", endpoint: "https://example.test/mcp",
  toolCount: 4, createdAt: "2026-10-24T00:00:00Z",
  updatedAt: "2026-10-24T00:00:00Z",
};

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { resetLocaleForTesting(); });

describe("Owned Tool service description presentation", () => {
  it.each([
    {
      kind: "agent" as const, pluginId: "io.mender.provider.agent-delegation",
      original: "Managed Agent Delegation tools for Grok",
      expected: "Agent「Grok」的受管委派工具",
    },
    {
      kind: "sandbox" as const, pluginId: "io.mender.provider.workspace",
      original: "Managed Workspace Tools for Grok",
      expected: "沙盒「Grok」的受管工作区工具",
    },
    {
      kind: "sandbox" as const, pluginId: "io.mender.provider.sandbox-jobs",
      original: "Owner-managed SSH Durable Jobs for Grok",
      expected: "沙盒「Grok」所属的 SSH 持久任务工具",
    },
  ])("translates only matching owner-generated $pluginId text", ({ kind, pluginId, original, expected }) => {
    const item: Provider = { ...base, toolSourceKind: kind, toolSourceName: "Grok", pluginId, description: original };
    expect(toolServiceDescription(item)).toBe(original);
    setLocale("zh-CN");
    expect(toolServiceDescription(item)).toBe(expected);
    expect(toolServiceDescription({ ...item, description: "Owner-customized description" }))
      .toBe("Owner-customized description");
    expect(toolServiceDescription({ ...item, toolSourceKind: "provider" })).toBe(original);
    expect(toolServiceDescription({ ...item, pluginId: "dev.example.plugin" })).toBe(original);
    expect(toolServiceDescription({ ...item, toolSourceName: "Another owner" })).toBe(original);
  });

  it("keeps user Provider descriptions even when they copy host wording", () => {
    setLocale("zh-CN");
    const original = "Managed Agent Delegation tools for Grok";
    const userProvider: Provider = { ...base, description: original, pluginId: "io.mender.provider.mcp" };
    expect(toolServiceDescription(userProvider)).toBe(original);
    expect(toolServiceDescription({ ...userProvider, description: "用户提供的中文说明" }))
      .toBe("用户提供的中文说明");
  });
});
