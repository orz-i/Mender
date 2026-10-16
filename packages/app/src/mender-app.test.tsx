import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
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
      "Agents",
      "Servers",
      "Policy",
      "Credentials",
      "Audit",
    ]) {
      expect(screen.getByRole("link", { name })).toBeTruthy();
    }
  });

  it("shows only the brand and route title without redundant descriptions", () => {
    render(
      <MemoryRouter>
        <MenderApp client={overviewClient()} />
      </MemoryRouter>,
    );

    const brand = document.querySelector('[data-slot="sidebar-brand"]');
    expect(brand?.textContent?.trim()).toBe("Mender");
    const header = screen.getByRole("banner");
    expect(within(header).getByRole("heading", { name: "Overview" })).toBeTruthy();
    expect(header.querySelector("p")).toBeNull();
    expect(screen.queryByText("Tool control plane")).toBeNull();

    const status = within(header).getByRole("status");
    expect(status).toBeTruthy();
    expect(status.querySelector("span[aria-hidden='true']")).toBeTruthy();

    fireEvent.click(within(header).getByRole("button", { name: "Language" }));
    expect(within(header).getByRole("heading", { name: "概览" })).toBeTruthy();
    expect(screen.queryByText("工具控制平面")).toBeNull();
    expect(brand?.textContent?.trim()).toBe("Mender");
  });

  it("shows one overview topic at a time and switches to read-only resource previews", async () => {
    render(
      <MemoryRouter>
        <MenderApp client={overviewClient()} />
      </MemoryRouter>,
    );

    expect(screen.getByText("No invocation records yet.")).toBeTruthy();
    expect(screen.queryByText("No providers")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Providers / Profiles" }));
    expect(await screen.findByText("No providers")).toBeTruthy();
    expect(await screen.findByText("No profiles")).toBeTruthy();
    expect(screen.queryByText("No invocation records yet.")).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Display name" })).toBeNull();
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

    const brand = document.querySelector('[data-slot="sidebar-brand"]');
    const collapse = screen.getByRole("button", { name: "Collapse sidebar" });
    expect(brand?.contains(collapse)).toBe(true);
    fireEvent.click(collapse);

    const expand = screen.getByRole("button", { name: "Expand sidebar" });
    const logoContainer = document.querySelector('[data-slot="sidebar-brand-logo"]');
    expect(brand?.contains(expand)).toBe(true);
    expect(logoContainer?.contains(expand)).toBe(true);
    expect(brand?.classList.contains("justify-center")).toBe(true);
    expect(expand.classList.contains("opacity-0")).toBe(true);
    expect(expand.classList.contains("group-hover/brand:opacity-100")).toBe(true);
    expect(screen.getByRole("link", { name: "Providers" })).toBeTruthy();
    expect(window.localStorage.getItem("mender.sidebar.collapsed")).toBe("true");

    fireEvent.click(expand);
    expect(brand?.contains(screen.getByRole("button", { name: "Collapse sidebar" }))).toBe(true);
    expect(window.localStorage.getItem("mender.sidebar.collapsed")).toBe("false");
  });

  it("switches the active UI to Simplified Chinese without a reload", () => {
    render(
      <MemoryRouter>
        <MenderApp client={overviewClient()} />
      </MemoryRouter>,
    );

    const topbar = within(screen.getByRole("banner"));
    const switchLanguage = topbar.getByRole("button", { name: "Language" });
    const connection = topbar.getByRole("status");
    expect(connection.compareDocumentPosition(switchLanguage) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(screen.getByRole("complementary")).queryByRole("combobox", { name: "Language" })).toBeNull();
    fireEvent.click(switchLanguage);

    expect(screen.getByRole("heading", { name: "概览" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "插件市场" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "提供方" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "技能" })).toBeTruthy();
    expect(
      screen.queryByText("本地控制平面状态与近期 Mender 活动。"),
    ).toBeNull();
    expect(window.localStorage.getItem("mender.locale")).toBe("zh-CN");
    expect(document.documentElement.lang).toBe("zh-CN");

    fireEvent.click(topbar.getByRole("button", { name: "语言" }));
    expect(screen.getByRole("heading", { name: "Overview" })).toBeTruthy();
    expect(window.localStorage.getItem("mender.locale")).toBe("en");
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
