import type { Agent } from "@mender/sdk";
import { t } from "../../i18n/i18n";

export type AgentBackend = Agent["backend"];

// Exhaustive registry: SDK Backend union changes must be explicitly reflected
// in every user-facing identity before TypeScript can compile.
export const agentBackendDetails = {
  codex: {
    name: "Codex",
    pluginId: "io.mender.agent.codex",
  },
  grok: {
    name: "Grok",
    pluginId: "io.mender.agent.grok",
  },
  opencode: {
    name: "OpenCode",
    pluginId: "io.mender.agent.opencode",
  },
  antigravity: {
    name: "Antigravity",
    pluginId: "io.mender.agent.antigravity",
  },
} as const satisfies Record<AgentBackend, {
  name: string;
  pluginId: string;
}>;

export const agentBackendOptions = Object.keys(agentBackendDetails) as AgentBackend[];

export function agentBackendLabel(backend:AgentBackend):string {
  return agentBackendDetails[backend].name;
}

export function agentBackendDescription(backend:AgentBackend):string {
  switch(backend) {
    case "codex": return t("Codex CLI requires the separately installed codex-acp adapter.");
    case "grok": return t("Grok CLI uses its built-in ACP stdio transport.");
    case "opencode": return t("OpenCode CLI uses its native ACP stdio transport.");
    case "antigravity": return t("Antigravity CLI uses its authenticated headless JSON stream; native permissions are not Mender ACP approvals.");
    default: {
      const exhaustive:never=backend;
      return exhaustive;
    }
  }
}

// A marketplace installation identifies a concrete Agent backend. Unknown or
// non-Agent plugin identities cannot be silently interpreted as Codex.
export function agentBackendForPluginId(pluginId:string):AgentBackend|null {
  for (const backend of agentBackendOptions) {
    if (agentBackendDetails[backend].pluginId === pluginId) return backend;
  }
  return null;
}
