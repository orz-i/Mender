import type { Provider } from "@mender/sdk";
import { expect, it } from "vitest";
import { toolServiceDescription } from "./tool-service-description";

const provider = { providerId: "prov_fixture", displayName: "Fixture", namespace: "fixture" } as Provider;
it("renders user-controlled external Provider description without reinterpretation", () => {
  expect(toolServiceDescription({...provider,description:"Document lookup service"})).toBe("Document lookup service");
  expect(toolServiceDescription({...provider,description:"Managed Agent Delegation tools for Grok"})).toBe("Managed Agent Delegation tools for Grok");
  expect(toolServiceDescription({...provider,description:""})).toBe("");
});
