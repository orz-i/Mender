import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import type { MarketplacePlugin } from "./marketplace";
import { marketplaceDescription } from "./marketplace-description";

function plugin(key: string, description: string, source: string): MarketplacePlugin {
  return {
    key, kind: "provider", kindLabel: "Provider plugin",
    displayName: "Fixture", description, source, sourceLabel: source,
    version: "1.0.0", status: "enabled", enabled: true,
    capabilities: [], actions: { enable: false, disable: false, remove: false },
    technicalDetails: [],
  };
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { resetLocaleForTesting(); });

describe("Host-reviewed Marketplace descriptions", () => {
  it.each([
    ["io.mender.provider.mcp@builtin", "Model Context Protocol provider"],
    ["io.mender.provider.openapi@builtin", "Expose supported OpenAPI operations as governed Mender Tools"],
    ["io.mender.provider.model-tools@builtin", "Bridge provider-hosted model tools into governed Mender Tools"],
    ["io.mender.sandbox.local-workspace@builtin", "Trusted host workspace backend; host or restricted-host, not container isolation"],
    ["io.mender.sandbox.docker@builtin", "Trusted local Docker Engine backend with monitored readiness and container isolation checks"],
    ["io.mender.sandbox.ssh-remote@builtin", "Pinned-host-key SSH remote account Workspace; remote is not OS/container isolation"],
    ["io.mender.sandbox.daytona@builtin", "Opt-in remote Sandbox backend; live integration is not accepted"],
    ["io.mender.agent.codex@builtin", "Codex CLI through the separately installed codex-acp adapter"],
    ["io.mender.agent.grok@builtin", "Grok CLI through its native ACP stdio protocol"],
    ["io.mender.agent.opencode@builtin", "OpenCode CLI through its native ACP v1 stdio protocol"],
    ["io.mender.agent.antigravity@builtin", "Google Antigravity CLI through its official headless JSON stream (not ACP)"],
    ["io.mender.tunnel.openai@builtin", "Trusted OpenAI tunnel-client transport; external control-plane readiness requires observation"],
  ])("translates only the exact trusted %s entry", (key, raw) => {
    const known = plugin(key, raw, "builtin");
    expect(marketplaceDescription(known)).toBe(raw);
    setLocale("zh-CN");
    expect(marketplaceDescription(known)).not.toBe(raw);
    expect(marketplaceDescription(known)).toMatch(/[一-龥]/u);
    expect(marketplaceDescription(plugin(key, raw, "local"))).toBe(raw);
    expect(marketplaceDescription(plugin(key, raw + " (updated)", "builtin"))).toBe(raw + " (updated)");
    expect(marketplaceDescription(plugin("dev.example.other@builtin", raw, "builtin"))).toBe(raw);
  });

  it("translates exact reviewed AnySearch metadata, not arbitrary curated text", () => {
    const text = "Search the web and retrieve pages through AnySearch's hosted MCP service.";
    const reviewed = plugin("integration:io.mender.integration.anysearch", text, "curated");
    setLocale("zh-CN");
    expect(marketplaceDescription(reviewed)).toBe("通过 AnySearch 托管的 MCP 服务搜索网页并获取页面内容");
    expect(marketplaceDescription(plugin(reviewed.key, "An updated description", "curated")))
      .toBe("An updated description");
    expect(marketplaceDescription(plugin(reviewed.key, text, "local"))).toBe(text);
  });
});
