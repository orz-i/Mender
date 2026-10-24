import { t } from "../../i18n/i18n";
import type { MarketplacePlugin } from "./marketplace";

/** Only exact host-reviewed text is localized. Extensions and changed descriptors retain their originals. */
const hostDescriptions: Record<string, {
  source: "builtin" | "curated";
  original: string;
  translated: () => string;
}> = {
  "io.mender.provider.mcp@builtin": {
    source: "builtin",
    original: "Model Context Protocol provider",
    translated: () => t("Model Context Protocol provider"),
  },
  "io.mender.provider.openapi@builtin": {
    source: "builtin",
    original: "Expose supported OpenAPI operations as governed Mender Tools",
    translated: () => t("Expose supported OpenAPI operations as governed Mender Tools"),
  },
  "io.mender.provider.model-tools@builtin": {
    source: "builtin",
    original: "Bridge provider-hosted model tools into governed Mender Tools",
    translated: () => t("Bridge provider-hosted model tools into governed Mender Tools"),
  },
  "io.mender.sandbox.local-workspace@builtin": {
    source: "builtin",
    original: "Trusted host workspace backend; host or restricted-host, not container isolation",
    translated: () => t("Trusted host workspace backend; host or restricted-host, not container isolation"),
  },
  "io.mender.sandbox.docker@builtin": {
    source: "builtin",
    original: "Trusted local Docker Engine backend with monitored readiness and container isolation checks",
    translated: () => t("Trusted local Docker Engine backend with monitored readiness and container isolation checks"),
  },
  "io.mender.sandbox.ssh-remote@builtin": {
    source: "builtin",
    original: "Pinned-host-key SSH remote account Workspace; remote is not OS/container isolation",
    translated: () => t("Pinned-host-key SSH remote account Workspace; remote is not OS/container isolation"),
  },
  "io.mender.sandbox.daytona@builtin": {
    source: "builtin",
    original: "Opt-in remote Sandbox backend; live integration is not accepted",
    translated: () => t("Opt-in remote Sandbox backend; live integration is not accepted"),
  },
  "io.mender.agent.codex@builtin": {
    source: "builtin",
    original: "Codex CLI through the separately installed codex-acp adapter",
    translated: () => t("Codex CLI through the separately installed codex-acp adapter"),
  },
  "io.mender.agent.grok@builtin": {
    source: "builtin",
    original: "Grok CLI through its native ACP stdio protocol",
    translated: () => t("Grok CLI through its native ACP stdio protocol"),
  },
  "io.mender.agent.opencode@builtin": {
    source: "builtin",
    original: "OpenCode CLI through its native ACP v1 stdio protocol",
    translated: () => t("OpenCode CLI through its native ACP v1 stdio protocol"),
  },
  "io.mender.agent.antigravity@builtin": {
    source: "builtin",
    original: "Google Antigravity CLI through its official headless JSON stream (not ACP)",
    translated: () => t("Google Antigravity CLI through its official headless JSON stream (not ACP)"),
  },
  "io.mender.tunnel.openai@builtin": {
    source: "builtin",
    original: "Trusted OpenAI tunnel-client transport; external control-plane readiness requires observation",
    translated: () => t("Trusted OpenAI tunnel-client transport; external control-plane readiness requires observation"),
  },
  "integration:io.mender.integration.anysearch": {
    source: "curated",
    original: "Search the web and retrieve pages through AnySearch's hosted MCP service.",
    translated: () => t("Search the web and retrieve pages through AnySearch's hosted MCP service."),
  },
};

export function marketplaceDescription(plugin: MarketplacePlugin): string {
  const known = hostDescriptions[plugin.key];
  return known !== undefined &&
    known.source === plugin.source &&
    known.original === plugin.description
      ? known.translated()
      : plugin.description;
}
