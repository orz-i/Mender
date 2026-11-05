import { useEffect, useState } from "react";

import type { Agent, ManagementClient, PluginCatalogueEntry } from "@mender/sdk";

import { errorMessage } from "../../shared/errors";
import { agentBackendDetails, type AgentBackend } from "./agent-backends";

export type AgentPresetSnapshot = {
  loaded: boolean;
  agents: Agent[] | null;
  plugins: PluginCatalogueEntry[] | null;
  agentError: string | null;
  catalogueError: string | null;
};

const initialSnapshot: AgentPresetSnapshot = {
  loaded: false,
  agents: null,
  plugins: null,
  agentError: null,
  catalogueError: null,
};

export function useAgentPresetCatalog(client: ManagementClient) {
  const [snapshot, setSnapshot] = useState<AgentPresetSnapshot>(initialSnapshot);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let active = true;
    void Promise.allSettled([
      Promise.resolve().then(() => client.listAgents()),
      Promise.resolve().then(() => client.listPluginCatalogue()),
    ]).then(([agentResult, catalogueResult]) => {
      if (!active) return;
      setSnapshot({
        loaded: true,
        agents: agentResult.status === "fulfilled" ? agentResult.value.agents : null,
        plugins: catalogueResult.status === "fulfilled" ? catalogueResult.value.plugins : null,
        agentError: agentResult.status === "rejected" ? errorMessage(agentResult.reason) : null,
        catalogueError: catalogueResult.status === "rejected" ? errorMessage(catalogueResult.reason) : null,
      });
    });
    return () => { active = false; };
  }, [client, revision]);

  return { snapshot, refresh: () => setRevision((current) => current + 1) };
}

export type AgentPresetState =
  | "configured"
  | "configured-unavailable"
  | "ready"
  | "unavailable"
  | "unknown";

export type AgentPresetEntry = {
  backend: AgentBackend;
  plugin: PluginCatalogueEntry | null;
  instances: Agent[];
  configuredCount: number | null;
  canConfigure: boolean;
  state: AgentPresetState;
  issue: string | null;
  destination: string;
};

export function projectAgentPreset(
  backend: AgentBackend,
  snapshot: AgentPresetSnapshot,
): AgentPresetEntry {
  const id = agentBackendDetails[backend].pluginId;
  const plugin = snapshot.plugins?.find((entry) =>
    entry.pluginId === id && entry.kind === "agent.backend" && entry.sourceKind === "builtin",
  ) ?? null;
  const instances = snapshot.agents?.filter((agent) => agent.backend === backend) ?? [];
  const configuredCount = snapshot.agents === null ? null : instances.length;
  const canConfigure = plugin?.enabled === true && plugin.status === "enabled" &&
    plugin.marketplaceAction === "create.agent" && !plugin.issue;

  const state: AgentPresetState =
    instances.some((agent) => agent.backendAvailable) ? "configured" :
      instances.length > 0 ? "configured-unavailable" :
        snapshot.agents === null || snapshot.plugins === null ? "unknown" :
          canConfigure ? "ready" : "unavailable";

  // Do not silently select the wrong workspace when a backend has multiple
  // configured resources or the resource inventory could not be read.
  const destination = instances.length === 1
    ? "/agents/" + encodeURIComponent(instances[0]!.agentId) + "?view=configuration"
    : instances.length > 1 || snapshot.agents === null || !canConfigure
      ? "/agents/backend/" + backend
      : "/agents/new?pluginId=" + encodeURIComponent(id) + "&from=agents";

  return {
    backend,
    plugin,
    instances,
    configuredCount,
    canConfigure,
    state,
    issue: plugin?.issue ?? (instances.find((agent) => agent.issue)?.issue ?? null),
    destination,
  };
}
