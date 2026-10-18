import type { Provider } from "@mender/sdk";

import { toolSourceLabel } from "../tools/tool-source-label";

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
