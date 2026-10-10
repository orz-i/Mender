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
      "Plugin Marketplace",
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

  it("keeps the quick navigation searchable and closes it after selecting a page", () => {
    render(
      <MemoryRouter>
        <MenderApp client={overviewClient()} />
      </MemoryRouter>,
    );

    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    const dialog = screen.getByRole("dialog", { name: "Quick navigation" });
    const search = screen.getByRole("textbox", { name: "Search pages" });
    fireEvent.change(search, { target: { value: "Overview" } });

    expect(screen.getByRole("button", { name: "Overview" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Providers" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Overview" }));

    expect(screen.queryByRole("dialog", { name: "Quick navigation" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Overview" })).toBeTruthy();
    expect(dialog.isConnected).toBe(false);
  });

  it("collapses and expands the sidebar without losing page navigation", () => {
    render(
      <MemoryRouter>
        <MenderApp client={overviewClient()} />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Collapse sidebar" }));
    expect(screen.getByRole("button", { name: "Expand sidebar" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Providers" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Expand sidebar" }));
    expect(screen.getByRole("button", { name: "Collapse sidebar" })).toBeTruthy();
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
    expect(screen.getByRole("link", { name: "插件市场" })).toBeTruthy();
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
