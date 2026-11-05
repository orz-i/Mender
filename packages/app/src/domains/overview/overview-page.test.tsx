import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { AuditRecord, ManagementClient } from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { OverviewPage } from "./overview-page";

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("OverviewPage load states", () => {
  it("shows a retriable error instead of fake zero metrics or empty activity", async () => {
    const getSystemStatus = vi.fn()
      .mockRejectedValueOnce(new Error("Runtime request failed"))
      .mockResolvedValue({ ready: true });
    const client = {
      getSystemStatus,
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
      listSkills: vi.fn().mockResolvedValue({ installations: [] }),
      listAudit: vi.fn().mockResolvedValue({ records: [] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [] }),
    } as unknown as ManagementClient;

    render(<MemoryRouter><OverviewPage client={client} /></MemoryRouter>);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Runtime request failed");
    expect(screen.queryByText("No invocation records yet.")).toBeNull();
    expect(document.querySelector('[data-slot="metric-strip"]')).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    await screen.findByText("Get started with tool providers");
    await waitFor(() => expect(getSystemStatus).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByText("No invocation records yet.")).toBeTruthy();
    expect(screen.getByRole("region", { name: "Workspace summary" })).toBeTruthy();
    expect(screen.getByText("No providers")).toBeTruthy();
    expect(screen.getByText("No profiles")).toBeTruthy();
    expect(document.querySelector('[data-slot="metric-strip"]')).toBeNull();
  });
});

const testProfile = {
  profileId: "prof_00000000000000000000000000000042",
  slug: "work",
  displayName: "Work Tools",
  mode: "hybrid" as const,
  tools: [],
};
const overviewAuditRecord: AuditRecord = {
  invocationId: "inv_overview_1",
  toolId: "tool_read",
  providerId: "prov_test",
  principalKind: "service",
  principalRef: "profile:" + testProfile.profileId,
  startedAt: "2026-10-24T10:00:00Z",
  finishedAt: "2026-10-24T10:00:02Z",
  policyKind: "allow",
  policyReason: "",
  approval: "not_required",
  status: "completed",
  resultKind: "complete",
  errorClass: "",
  errorSummary: "",
  origin: "mcp",
  scopeKind: "profile",
  scopeReference: testProfile.profileId,
  outcomeKind: "complete",
  rejectionCode: "",
  failureSource: "",
  failureCode: "",
};

function mountOverviewWithRecords(records: AuditRecord[]) {
  const client = {
    getSystemStatus: vi.fn().mockResolvedValue({ ready: true }),
    listProviders: vi.fn().mockResolvedValue({ providers: [{ providerId: "prov_test" }] }),
    listProfiles: vi.fn().mockResolvedValue({ profiles: [testProfile] }),
    listSkills: vi.fn().mockResolvedValue({ installations: [] }),
    listAudit: vi.fn().mockResolvedValue({ records }),
    browseTools: vi.fn().mockResolvedValue({ tools: [{
      toolId: "tool_read", title: "Read Agent task result", name: "read_task_result",
    }] }),
  } as unknown as ManagementClient;
  render(<MemoryRouter><OverviewPage client={client} /></MemoryRouter>);
  return client;
}

describe("Overview readable invocation preview", () => {
  it("shows known Profile names and translated outcomes without losing exact audit identity", async () => {
    setLocale("zh-CN");
    mountOverviewWithRecords([overviewAuditRecord]);
    expect(await screen.findByText("配置档 · Work Tools")).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "调用方" })).toBeTruthy();
    const caller = screen.getByText("配置档 · Work Tools");
    expect(caller.getAttribute("title")).toBe("service:profile:" + testProfile.profileId);
    expect(screen.getByText("已完成")).toBeTruthy();
    expect(screen.queryByText("completed")).toBeNull();
    expect(screen.queryByText("service:profile:" + testProfile.profileId)).toBeNull();
  });

  it("exposes a direct Audit record destination and an explicit recent-failure workflow", async () => {
    setLocale("en");
    const failure = { ...overviewAuditRecord, invocationId: "inv_failed_path", status: "failed" as const };
    mountOverviewWithRecords([overviewAuditRecord, failure]);
    const deepLinks = await screen.findAllByRole("link", { name: /View invocation/ });
    expect(deepLinks).toHaveLength(2);
    const failedLink = deepLinks.find((link) => link.getAttribute("href") === "/audit?q=inv_failed_path");
    expect(failedLink).toBeTruthy();
    expect(failedLink?.textContent).toContain("Read Agent task result");
    const attention = screen.getByRole("link", { name: "1 failures in recent calls" });
    expect(attention.getAttribute("href")).toBe("/audit?status=attention");
  });

  it("does not guess unknown identities and keeps distinct short hints and original values", async () => {
    setLocale("en");
    mountOverviewWithRecords([
      { ...overviewAuditRecord, invocationId: "old", toolId: "unknown_a",
        principalRef: "profile:prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", status: "failed" },
      { ...overviewAuditRecord, invocationId: "local", toolId: "unknown_b",
        principalKind: "local_user", principalRef: "os:501", status: "denied" },
      { ...overviewAuditRecord, invocationId: "service", toolId: "unknown_c",
        principalKind: "service", principalRef: "service:external-connection-1", status: "queued" },
    ]);
    const unknown = await screen.findByText("Profile · …aaaaaaaa");
    expect(unknown.getAttribute("title")).toBe("service:profile:prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
    const local = screen.getByText("Local user · 501");
    expect(local.getAttribute("title")).toBe("local_user:os:501");
    const external = screen.getByText(/^Service · …/);
    expect(external.getAttribute("title")).toBe("service:service:external-connection-1");
    expect(screen.getByText("Failed")).toBeTruthy();
    expect(screen.getByText("Denied")).toBeTruthy();
    expect(screen.getByText("queued")).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Caller" })).toBeTruthy();
  });
});
