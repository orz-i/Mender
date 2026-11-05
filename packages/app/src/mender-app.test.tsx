import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { LocalOwnerCapabilities, ManagementClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "./i18n/i18n";
import { MenderApp } from "./mender-app";

beforeEach(() => {
  window.localStorage.clear();
  document.documentElement.classList.remove("dark");
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  window.localStorage.clear();
  document.documentElement.classList.remove("dark");
  resetLocaleForTesting();
});

function mount(client = overviewClient(), localOwner?: LocalOwnerCapabilities, path = "/") {
  render(<MemoryRouter initialEntries={[path]}><MenderApp client={client}
    {...(localOwner ? { localOwner } : {})} /></MemoryRouter>);
}

describe("MenderApp Magpie-inspired native workspace", () => {
  it("shows only the principal destinations in one horizontal navigation and keeps all resources reachable", () => {
    mount();
    expect(screen.getByRole("heading", { name: "Overview", level: 1 })).toBeTruthy();
    const top = screen.getByRole("navigation", { name: "Primary navigation" });
    for (const name of ["Overview", "Tools", "Profiles", "Agents", "Sandboxes", "Skills", "Plugin Marketplace", "Audit"]) {
      expect(within(top).getByRole("link", { name })).toBeTruthy();
    }
    expect(document.querySelector("aside")).toBeNull();
    expect(document.querySelector('[data-slot="management-layout"]')?.getAttribute("data-layout")).toBe("top");
    expect(within(top).queryByRole("link", { name: "Runtime" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Collapse sidebar" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Servers" })).toBeNull();
  });

  it("puts uncommon operational settings behind the single Settings menu rather than duplicating navigation rows", async () => {
    mount();
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    for (const name of ["Tunnels", "Credentials", "Policy", "Runtime"]) {
      expect(await screen.findByRole("button", { name })).toBeTruthy();
    }
    expect(screen.queryByRole("button", { name: "Approvals" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Runtime" }));
    expect(screen.getByRole("heading", { name: "Runtime" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Tunnels" })).toBeNull();
  });

  it("defaults to the supplied light reference and persists explicit dark and system appearance", () => {
    mount();
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    const appearance = screen.getByRole("group", { name: "Appearance" });
    fireEvent.click(within(appearance).getByRole("button", { name: "Dark" }));
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(window.localStorage.getItem("mender.appearance")).toBe("dark");
    fireEvent.click(within(appearance).getByRole("button", { name: "Light" }));
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(window.localStorage.getItem("mender.appearance")).toBe("light");
  });

  it("preserves owner-only approval status and opens pending requests without a side navigation", async () => {
    const approvalId = "aapr_0123456789abcdef0123456789abcdef";
    const taskId = "atask_0123456789abcdef0123456789abcdef";
    const agentId = "agent_0123456789abcdef0123456789abcdef";
    const request = {
      agentName: "Owner review Agent", workspaceKey: "",
      approval: {
        approvalId, taskId, agentId, kind: "read" as const, target: "review.txt",
        canAllow: true, status: "pending" as const,
        createdAt: "2026-10-17T00:00:00Z", expiresAt: "2099-01-01T00:00:00Z",
      },
    };
    const localOwner: LocalOwnerCapabilities = {
      approvals: {
        listAgentApprovals: vi.fn().mockResolvedValue({ approvals: [] }),
        listPendingAgentApprovals: vi.fn().mockResolvedValue({ approvals: [request] }),
        decideAgentApproval: vi.fn().mockResolvedValue({ ...request.approval, status: "allowed_once" }),
        getAgentApprovalPolicy: vi.fn(),
        listAgentApprovalPolicies: vi.fn(),
        setAgentApprovalPolicy: vi.fn(),
        releasePendingAgentApprovals: vi.fn(),
      },
    };
    mount(overviewClient(), localOwner);
    const header = within(screen.getByRole("banner"));
    expect(await header.findByLabelText("Pending approvals: 1")).toBeTruthy();
    fireEvent.click(header.getByRole("button", { name: "Pending approvals" }));
    expect(await screen.findByText("review.txt")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Approvals" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Allow once" })).toBeTruthy();
  });

  it("provides a complete mobile menu and closes it on navigation", () => {
    mount({
      ...overviewClient(),
      listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: [] }),
      listIntegrationOfferings: vi.fn().mockResolvedValue({ integrations: [] }),
    });
    fireEvent.click(screen.getByRole("button", { name: "Open navigation" }));
    const menu = screen.getByRole("dialog", { name: "Mender" });
    expect(within(menu).getByRole("link", { name: "Profiles" })).toBeTruthy();
    expect(within(menu).getByRole("link", { name: "Runtime" })).toBeTruthy();
    fireEvent.click(within(menu).getByRole("link", { name: "Plugin Marketplace" }));
    expect(screen.queryByRole("dialog", { name: "Mender" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Plugin Marketplace" })).toBeTruthy();
  });

  it("keeps a precise parent breadcrumb and current route accessible on creation", () => {
    mount(overviewClient(), undefined, "/profiles/new");
    const trail = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(trail).getByRole("link", { name: "Profiles" })).toBeTruthy();
    expect(within(trail).getByText("Create profile")).toBeTruthy();
    fireEvent.click(within(trail).getByRole("link", { name: "Profiles" }));
    expect(screen.getByRole("heading", { name: "Profiles" })).toBeTruthy();
  });

  it("preserves the page search and Ctrl/Cmd+K without adding another visible toolbar", () => {
    mount();
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    const dialog = screen.getByRole("dialog", { name: "Quick navigation" });
    const search = within(dialog).getByRole("textbox", { name: "Search pages" });
    fireEvent.change(search, { target: { value: "Runtime" } });
    expect(within(dialog).getByRole("button", { name: "Runtime" })).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Runtime" }));
    expect(screen.queryByRole("dialog", { name: "Quick navigation" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Runtime" })).toBeTruthy();
  });

  it("switches language via Settings without losing route or the new single-row chrome", () => {
    mount();
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    fireEvent.click(screen.getByRole("button", { name: /Language/ }));
    expect(screen.getByRole("heading", { name: "概览" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "插件市场" })).toBeTruthy();
    expect(window.localStorage.getItem("mender.locale")).toBe("zh-CN");
    expect(document.documentElement.lang).toBe("zh-CN");
    fireEvent.click(screen.getByRole("button", { name: /语言/ }));
    expect(screen.getByRole("heading", { name: "Overview" })).toBeTruthy();
  });

  it("renders real empty overview resources and activity without fake metrics", async () => {
    mount();
    expect(await screen.findByText("Get started with tool providers")).toBeTruthy();
    expect(screen.getByText("No providers")).toBeTruthy();
    expect(screen.getByText("No profiles")).toBeTruthy();
    expect(screen.getByText("No invocation records yet.")).toBeTruthy();
    expect(screen.getByRole("region", { name: "Workspace summary" })).toBeTruthy();
  });
});

function overviewClient(): ManagementClient {
  return {
    getSystemStatus: () => Promise.resolve({
      ready: true,
      policy: {
        mode: "configured", editable: true,
        approval: "fail_closed_when_required", defaultDecision: "allow",
        ruleCount: 0, enabledRuleCount: 0,
      },
      credentials: { resolver: "inventory", persistentStore: true, editable: true },
      mcp: { stdio: true, streamableHttp: true, exposureModes: ["direct", "dynamic", "hybrid"] },
    }),
    listProviders: () => Promise.resolve({ providers: [] }),
    listProfiles: () => Promise.resolve({ profiles: [] }),
    listSkills: () => Promise.resolve({ installations: [] }),
    listAudit: () => Promise.resolve({ records: [] }),
  } as unknown as ManagementClient;
}
