import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { resetLocaleForTesting, setLocale } from "./i18n/i18n";
import { MenderApp } from "./mender-app";

beforeEach(() => {
  window.localStorage.clear();
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  window.localStorage.clear();
  resetLocaleForTesting();
});

describe("MenderApp", () => {
  it("renders the management navigation and overview", () => {
    render(
      <MemoryRouter>
        <MenderApp client={overviewClient()} />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "Overview" })).toBeTruthy();

    for (const name of [
      "Overview",
      "Providers",
      "Skills",
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

  it("switches the active UI to Simplified Chinese without a reload", () => {
    render(
      <MemoryRouter>
        <MenderApp client={overviewClient()} />
      </MemoryRouter>,
    );

    fireEvent.change(screen.getByRole("combobox", { name: "Language" }), {
      target: { value: "zh-CN" },
    });

    expect(screen.getByRole("heading", { name: "概览" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "提供方" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "技能" })).toBeTruthy();
    expect(
      screen.queryByText("本地控制平面状态与近期 Mender 活动。"),
    ).toBeNull();
    expect(window.localStorage.getItem("mender.locale")).toBe("zh-CN");
    expect(document.documentElement.lang).toBe("zh-CN");
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
    listSkills: () => Promise.resolve({ installations: [] }),
    listAudit: () => Promise.resolve({ records: [] }),
  } as unknown as MenderClient;
}
