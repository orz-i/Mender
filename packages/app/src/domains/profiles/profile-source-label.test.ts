import type { Agent,Provider,ToolSummary } from "@mender/sdk";
import { expect, it } from "vitest";

import { profileSourceLabels,profileToolSources } from "./profile-source-label";

it("disambiguates only colliding source names with stable configured namespaces", () => {
  const base = { displayName: "Example", namespace: "example", toolSourceName: "Example", toolSourceKind: "provider" as const };
  const sources = [
    { ...base, providerId: "prov_a", namespace: "agent-a" },
    { ...base, providerId: "prov_b", namespace: "agent-b" },
    { ...base, providerId: "prov_c", displayName: "AnySearch", toolSourceName: "AnySearch", toolSourceKind: "provider" as const },
  ] as Provider[];
  const labels = profileSourceLabels(sources);
  expect(labels.get("prov_a")).toBe("Example · agent-a");
  expect(labels.get("prov_b")).toBe("Example · agent-b");
  expect(labels.get("prov_c")).toBe("AnySearch");
});

it("derives Agent/native source identities independently of external Provider ancestry",()=>{
 const agents=[{agentId:"agent_11111111111111111111111111111111",name:"Reviewer"},{agentId:"agent_22222222222222222222222222222222",name:"Reviewer"}] as Agent[];
 const tools=[{toolId:"tool_native",sourceId:"native:workspace",sourceKind:"native"}] as ToolSummary[];
 const result=profileToolSources([],agents,tools);
 expect(result.map(s=>s.sourceId)).toEqual(["agent:"+agents[0]!.agentId,"agent:"+agents[1]!.agentId,"native:workspace"]);
 expect(result[0]?.label).toBe("Reviewer · 11111111");
 expect(result[1]?.label).toBe("Reviewer · 22222222");
 expect(result[2]?.toolCount).toBe(1);
});
