import { describe, expect, it } from "vitest";

import { getPageTrail } from "./page-chrome";

describe("getPageTrail", () => {
  it("keeps parent routes clickable and preserves the edit path", () => {
    expect(getPageTrail("/profiles/work/edit", { to: "/profiles", label: "Profiles" })).toEqual([
      { to: "/profiles", label: "Profiles" },
      { to: "/profiles/work", label: "work" },
      { to: "/profiles/work/edit", label: "Edit profile" },
    ]);
  });

  it("names create destinations and decodes exact plugin identifiers", () => {
    expect(getPageTrail("/agents/new", { to: "/agents", label: "Agents" }).at(-1)?.label)
      .toBe("Configure agent");
    expect(getPageTrail("/plugins/io.mender.provider.mcp%40builtin", { to: "/plugins", label: "Plugin Marketplace" }).at(-1)?.label)
      .toBe("io.mender.provider.mcp@builtin");
  });

  it("keeps Agent backend selection as one semantic destination", () => {
    expect(getPageTrail("/agents/backend/antigravity", { to: "/agents", label: "Agents" })).toEqual([
      { to: "/agents", label: "Agents" },
      { to: "/agents/backend/antigravity", label: "Agent configuration" },
    ]);
  });

  it("treats malformed encoded identifiers as safe text rather than throwing", () => {
    expect(getPageTrail("/plugins/invalid%ZZ", { to: "/plugins", label: "Plugin Marketplace" }).at(-1)?.label)
      .toBe("invalid%ZZ");
  });

  it("keeps an overview as a single root", () => {
    expect(getPageTrail("/", { to: "/", label: "Overview" })).toEqual([
      { to: "/", label: "Overview" },
    ]);
  });
});
