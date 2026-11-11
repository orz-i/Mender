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

  it("goes from actual overview failures to filtered audit and exact invocation results", async () => {
    const base = {
      toolId: "tool_search", providerId: "prov_search", principalKind: "service",
      principalRef: "profile:prof_test", startedAt: "2026-11-05T08:00:00Z",
      finishedAt: "2026-11-05T08:00:01Z", policyKind: "allow",
      policyReason: "", approval: "not_required", resultKind: "complete",
      errorClass: "", errorSummary: "", origin: "mcp", scopeKind: "profile",
      scopeReference: "prof_test", outcomeKind: "complete", rejectionCode: "",
      failureSource: "", failureCode: "",
    };
    const client = {
      ...overviewClient(),
      listAudit: vi.fn().mockResolvedValue({ records: [
        { ...base, invocationId: "inv_failure", status: "failed", errorClass: "execution" },
        { ...base, invocationId: "inv_success", status: "completed" },
      ] }),
      listProviderActivationAudit: vi.fn().mockResolvedValue({ records: [] }),
      listProfiles: vi.fn().mockResolvedValue({ profiles: [{
        profileId: "prof_test", slug: "work", displayName: "Work Tools", tools: [],
      }] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [{
        toolId: "tool_search", title: "Search", name: "search",
      }] }),
    } as unknown as ManagementClient;
    mount(client);
    const attention = await screen.findByRole("link", { name: "1 failures in recent calls" });
    fireEvent.click(attention);
    const table = await screen.findByRole("table", { name: "Invocation history" });
    expect(within(table).getByText("Failed")).toBeTruthy();
    expect(within(table).queryByText("Completed")).toBeNull();
    expect(within(table).getAllByRole("row")).toHaveLength(2);
    fireEvent.click(screen.getByRole("link", { name: "Overview" }));
    const deepLinks = await screen.findAllByRole("link", {
      name: /View invocation: Search/,
    });
    const failedLink = deepLinks.find((link) => link.getAttribute("href") === "/audit?q=inv_failure");
    expect(failedLink).toBeTruthy();
    fireEvent.click(failedLink!);
    expect(await screen.findByRole("table", { name: "Invocation history" })).toBeTruthy();
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Search audit records" }).value).toBe("inv_failure");
    expect(within(screen.getByRole("table", { name: "Invocation history" })).getAllByRole("row")).toHaveLength(2);
  });

  it("renders real empty overview resources and activity without fake metrics", async () => {
    mount();
    expect(await screen.findByText("Get started with tool providers")).toBeTruthy();
    expect(screen.getByText("No providers")).toBeTruthy();
    expect(screen.getByText("No profiles")).toBeTruthy();
    expect(screen.getByText("No invocation records yet.")).toBeTruthy();
    expect(screen.getByRole("region", { name: "Workspace summary" })).toBeTruthy();
  });

  it("mounts Desktop Workbench shell with custom titlebar, activity bar rail, and status bar", async () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <MenderApp
          variant="workbench"
          client={overviewClient()}
          localOwner={{
            approvals: {
              listPendingAgentApprovals: () => Promise.resolve({ approvals: [] }),
              listAgentApprovals: () => Promise.resolve({ approvals: [] }),
              decideAgentApproval: () => Promise.resolve({} as never),
              getAgentApprovalPolicy: () => Promise.resolve({} as never),
              listAgentApprovalPolicies: () => Promise.resolve({ policies: [] }),
              setAgentApprovalPolicy: () => Promise.resolve({} as never),
              releasePendingAgentApprovals: () => Promise.resolve({ released: 0 }),
            },
          }}
        />
      </MemoryRouter>,
    );

    expect(document.documentElement.dataset.platform).toBe("desktop");
    expect(document.querySelector('[data-slot="workbench-layout"]')?.getAttribute("data-layout")).toBe("workbench");
    expect(document.querySelector('[data-slot="workbench-titlebar"]')).toBeTruthy();
    expect(screen.getByText("Workbench")).toBeTruthy();

    const rail = screen.getByRole("navigation", { name: "Primary navigation" });
    expect(rail.getAttribute("data-slot")).toBe("workbench-activity-bar");
    for (const name of ["Overview", "Tools", "Profiles", "Agents", "Sandboxes", "Skills", "Plugin Marketplace", "Audit"]) {
      expect(within(rail).getByRole("link", { name })).toBeTruthy();
    }

    const statusBar = document.querySelector('[data-slot="workbench-statusbar"]');
    expect(statusBar).toBeTruthy();
    expect(await within(statusBar as HTMLElement).findByText("Runtime: Connected")).toBeTruthy();
    expect(within(statusBar as HTMLElement).getByText("Local Owner: Active")).toBeTruthy();
  });

  it("navigates and triggers quick command palette in Workbench shell", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <MenderApp variant="workbench" client={overviewClient()} />
      </MemoryRouter>,
    );

    const searchBtn = screen.getByRole("button", { name: "Search pages…" });
    fireEvent.click(searchBtn);
    expect(screen.getByRole("heading", { name: "Quick navigation" })).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "Search pages" }), { target: { value: "Runtime" } });
    const runtimeOption = screen.getByRole("button", { name: "Runtime" });
    fireEvent.click(runtimeOption);
    expect(screen.getByRole("heading", { name: "Runtime" })).toBeTruthy();
  });

  it("toggles the desktop bottom console drawer in Workbench shell", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <MenderApp variant="workbench" client={overviewClient()} />
      </MemoryRouter>,
    );

    expect(document.querySelector('[data-slot="workbench-console"]')).toBeNull();
    const toggleBtn = screen.getByRole("button", { name: "Toggle Console" });
    fireEvent.click(toggleBtn);
    expect(document.querySelector('[data-slot="workbench-console"]')).toBeTruthy();
    expect(screen.getByText("Console Logs")).toBeTruthy();
    expect(screen.getByText("Workbench initialized")).toBeTruthy();

    const closeBtn = screen.getByRole("button", { name: "Close console" });
    fireEvent.click(closeBtn);
    expect(document.querySelector('[data-slot="workbench-console"]')).toBeNull();
  });

  it("renders desktop window controls and shortcut indicators when provided", () => {
    const minimise = vi.fn().mockResolvedValue(undefined);
    const toggleMaximise = vi.fn().mockResolvedValue(undefined);
    const close = vi.fn().mockResolvedValue(undefined);

    render(
      <MemoryRouter initialEntries={["/"]}>
        <MenderApp
          variant="workbench"
          client={overviewClient()}
          windowControls={{ minimise, toggleMaximise, close }}
        />
      </MemoryRouter>,
    );

    const minBtn = screen.getByRole("button", { name: "Minimise" });
    const maxBtn = screen.getByRole("button", { name: "Toggle maximise" });
    const closeBtn = screen.getByRole("button", { name: "Close window" });

    fireEvent.click(minBtn);
    expect(minimise).toHaveBeenCalledTimes(1);

    fireEvent.click(maxBtn);
    expect(toggleMaximise).toHaveBeenCalledTimes(1);

    fireEvent.click(closeBtn);
    expect(close).toHaveBeenCalledTimes(1);

    expect(screen.getByText("⌘K Palette · ⌘` Console · ⌘⇧P Playground")).toBeTruthy();
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
