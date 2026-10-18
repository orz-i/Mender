import type { Provider } from "@mender/sdk";
import { expect, it } from "vitest";

import { profileSourceLabels } from "./profile-source-label";

it("disambiguates only colliding source names with stable configured namespaces", () => {
  const base = { displayName: "Agent", namespace: "agent", toolSourceName: "Agent", toolSourceKind: "agent" as const };
  const sources = [
    { ...base, providerId: "prov_a", namespace: "agent-a" },
    { ...base, providerId: "prov_b", namespace: "agent-b" },
    { ...base, providerId: "prov_c", displayName: "AnySearch", toolSourceName: "AnySearch", toolSourceKind: "provider" as const },
  ] as Provider[];
  const labels = profileSourceLabels(sources);
  expect(labels.get("prov_a")).toBe("Agent · agent-a");
  expect(labels.get("prov_b")).toBe("Agent · agent-b");
  expect(labels.get("prov_c")).toBe("AnySearch");
});
