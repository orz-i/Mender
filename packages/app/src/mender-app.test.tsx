import { render, screen } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { MenderApp } from "./mender-app";

describe("MenderApp", () => {
  it("renders the management navigation and overview", () => {
    render(
      <MemoryRouter>
        <MenderApp client={overviewClient()} />
      </MemoryRouter>,
    );

    expect(
      screen.getByRole("heading", { name: "Overview" }),
    ).toBeTruthy();

    for (const name of [
      "Overview",
      "Providers",
      "Catalog",
      "Profiles",
      "Servers",
      "Policy",
      "Credentials",
      "Audit",
    ]) {
      expect(screen.getByRole("link", { name })).toBeTruthy();
    }
  });
});

function overviewClient(): MenderClient {
  return {
    getSystemStatus: () => Promise.resolve({
      ready: true,
      policy: {
        mode: "configured",
        editable: true,
        approval: "fail_closed_when_required",
        defaultDecision: "allow",
        ruleCount: 0,
        enabledRuleCount: 0,
      },
      credentials: {
        resolver: "inventory",
        persistentStore: true,
        editable: true,
      },
      mcp: {
        stdio: true,
        streamableHttp: true,
        exposureModes: ["direct", "dynamic", "hybrid"],
      },
    }),
    listProviders: () => Promise.resolve({ providers: [] }),
    listProfiles: () => Promise.resolve({ profiles: [] }),
    listAudit: () => Promise.resolve({ records: [] }),
  } as unknown as MenderClient;
}
