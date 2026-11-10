import type { Agent, Provider, ToolSummary } from "@mender/sdk";
import { t } from "../../i18n/i18n";

import { toolSourceLabel } from "../tools";

/**
 * A human source label isn't a configured service identity. Disambiguate
 * collisions with the Provider namespace while keeping ProviderID as the key.
 * Precompute once per provider inventory to keep large source pickers linear.
 */
export function profileSourceLabels(providers: Provider[]): Map<string, string> {
  const counts = new Map<string, number>();
  for (const provider of providers) {
    const name = toolSourceLabel(provider);
    counts.set(name, (counts.get(name) ?? 0) + 1);
  }
  const labels = new Map<string, string>();
  for (const provider of providers) {
    const name = toolSourceLabel(provider);
    labels.set(
      provider.providerId,
      (counts.get(name) ?? 0) > 1 ? name + " · " + provider.namespace : name,
    );
  }
  return labels;
}

// The Profile chooser consumes real Tool SourceRefs. Provider inventory alone
// is insufficient: Agent as Tool and host-native Workspace/Jobs tools do not
// have (or create) a ProviderID.
export interface ProfileToolSource {
  sourceId: string;
  label: string;
  toolCount: number;
  selectable: boolean;
}

export function profileToolSources(providers: Provider[], agents: Agent[], tools: ToolSummary[]): ProfileToolSource[] {
  const providerLabels = profileSourceLabels(providers);
  const result: ProfileToolSource[] = providers.map((provider) => ({
    sourceId: "provider:" + provider.providerId,
    label: providerLabels.get(provider.providerId) ?? provider.displayName,
    toolCount: provider.toolCount,
    selectable: provider.toolSourceSelectable === true,
  }));
  const agentNames = new Map<string,number>();
  for (const agent of agents) agentNames.set(agent.name,(agentNames.get(agent.name)??0)+1);
  for (const agent of agents) result.push({
    sourceId: "agent:" + agent.agentId,
    label: (agentNames.get(agent.name)??0)>1 ? agent.name+" · "+agent.agentId.slice(-8) : agent.name,
    toolCount: 5,
    selectable: true, // AgentGrant/ToolBinding is independently required for invocation.
  });
  for (const native of [{id:"native:workspace",label:t("Workspace tools")},{id:"native:jobs",label:t("SSH Jobs")}] as const) {
    const count = tools.filter((tool) => tool.sourceId === native.id).length;
    if (count) result.push({sourceId:native.id,label:native.label,toolCount:count,selectable:true});
  }
  return result;
}
