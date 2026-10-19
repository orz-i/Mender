import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { AuditRecord, MenderClient, ProviderActivationAuditRecord } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { AuditPage } from "./audit-page";

const invocation: AuditRecord = {
  invocationId: "inv_complete",
  toolId: "tool_search",
  providerId: "provider_one",
  principalKind: "legacy",
  principalRef: "desktop",
  startedAt: "2026-09-26T23:26:00Z",
  finishedAt: "2026-09-26T23:26:02Z",
  policyKind: "allow",
  policyReason: "policy.good",
  approval: "not_required",
  status: "completed",
  resultKind: "complete",
  errorClass: "",
  errorSummary: "",
  origin: "desktop",
  scopeKind: "catalog",
  scopeReference: "",
  outcomeKind: "complete",
  rejectionCode: "",
  failureSource: "",
  failureCode: "",
};

const activation: ProviderActivationAuditRecord = {
  activationId: "act_denied",
  providerId: "provider_secure",
  action: "provider.activate",
  principalKind: "service",
  principalRef: "service-1",
  origin: "cli",
  scopeKind: "profile",
  scopeReference: "profile_1",
  startedAt: "2026-09-26T23:25:00Z",
  finishedAt: "2026-09-26T23:25:02Z",
  policyKind: "deny",
  policyReason: "policy.blocked",
  approval: "denied",
  status: "denied",
  errorClass: "policy",
  errorSummary: "Activation refused by policy",
};

function mount(
  invocations: AuditRecord[] = [invocation],
  activations: ProviderActivationAuditRecord[] = [activation],
  overrides: Partial<MenderClient> = {},
) {
  const listAudit = vi.fn().mockResolvedValue({ records: invocations });
  const listProviderActivationAudit = vi.fn().mockResolvedValue({ records: activations });
  const client = { listAudit, listProviderActivationAudit, ...overrides } as unknown as MenderClient;
  render(
    <MemoryRouter>
      <TooltipProvider>
        <AuditPage client={client} />
      </TooltipProvider>
    </MemoryRouter>,
  );
  return { listAudit, listProviderActivationAudit };
}

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});
afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("Audit workbench", () => {
  it("shows only one history type per viewport with localized semantic statuses", async () => {
    mount();
    const invocationTable = await screen.findByRole("table", { name: "Invocation history" });
    expect(within(invocationTable).getByText("Completed")).toBeTruthy();
    expect(within(invocationTable).getByText("Allow")).toBeTruthy();
    expect(within(invocationTable).queryByText("completed")).toBeNull();
    expect(screen.queryByRole("table", { name: "Provider activation history" })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Display name" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Provider activation history" }));
    const activationTable = screen.getByRole("table", { name: "Provider activation history" });
    expect(within(activationTable).getByText("Denied")).toBeTruthy();
    expect(within(activationTable).getByText("Deny")).toBeTruthy();
    expect(within(activationTable).getByText("provider_secure")).toBeTruthy();
    expect(screen.queryByRole("table", { name: "Invocation history" })).toBeNull();
    expect(screen.getByRole("button", { name: "Provider activation history" }).getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Invocation history" }));
    expect(screen.getByRole("table", { name: "Invocation history" })).toBeTruthy();
    expect(screen.queryByRole("table", { name: "Provider activation history" })).toBeNull();
  });

  it("filters only the active preview, clears no-matches, and keeps pagination usable", async () => {
    const many = Array.from({ length: 12 }, (_, index): AuditRecord => ({
      ...invocation,
      invocationId: `inv_${String(index).padStart(2, "0")}`,
      toolId: `search_${index}`,
      principalRef: index === 11 ? "special" : "desktop",
      status: index === 11 ? "failed" : "completed",
      errorClass: index === 11 ? "execution" : "",
      errorSummary: index === 11 ? "Safe diagnostic" : "",
    }));
    mount(many, []);
    expect(await screen.findByRole("table", { name: "Invocation history" })).toBeTruthy();

    expect(screen.getByText("Page 1 of 2")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Audit next page" }));
    expect(screen.getByText("Page 2 of 2")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "Search audit records" }), { target: { value: "special" } });
    expect(screen.queryByRole("navigation", { name: "Audit pagination" })).toBeNull();
    expect(within(screen.getByRole("table")).getByText("Failed")).toBeTruthy();
    expect(within(screen.getByRole("table")).queryByText("Completed")).toBeNull();

    fireEvent.change(screen.getByRole("textbox", { name: "Search audit records" }), { target: { value: "no matches" } });
    expect(await screen.findByText("No matching audit records")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(screen.getByRole("table")).toBeTruthy();

    fireEvent.click(screen.getByRole("combobox", { name: "Audit status" }));
    const failedOption = await screen.findByRole("option", { name: "Failed" });
    fireEvent.pointerDown(failedOption, { pointerId: 1, pointerType: "mouse", button: 0, isPrimary: true });
    fireEvent.pointerUp(failedOption, { pointerId: 1, pointerType: "mouse", button: 0, isPrimary: true });
    fireEvent.click(failedOption);
    await waitFor(() => {
      expect(within(screen.getByRole("table")).getByText("Failed")).toBeTruthy();
      expect(within(screen.getByRole("table")).queryByText("Completed")).toBeNull();
    });
  });

  it("searches the localized Chinese status shown in the audit preview", async () => {
    setLocale("zh-CN");
    mount([{ ...invocation, status: "failed", errorClass: "execution" }, invocation], []);
    expect(await screen.findByText("失败")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "搜索审计记录" }), {
      target: { value: "失败" },
    });
    const table = screen.getByRole("table", { name: "调用历史" });
    expect(within(table).getByText("失败")).toBeTruthy();
    expect(within(table).queryByText("已完成")).toBeNull();
  });

  it("opens failures directly on bounded diagnostics, then reveals identity and governance on demand", async () => {
    mount([{ ...invocation, status: "failed", errorClass: "execution", errorSummary: "", failureSource: "provider", failureCode: "provider_error" }], []);
    expect(await screen.findByText("Failed")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "View audit record" }));
    const dialog = within(screen.getByRole("dialog", { name: "Invocation details" }));
    expect(dialog.getByRole("button", { name: "Diagnostics" }).getAttribute("aria-pressed")).toBe("true");
    expect(dialog.getByText(/No diagnostic summary was retained/)).toBeTruthy();
    expect(dialog.getByText("provider_error")).toBeTruthy();
    expect(dialog.queryByText("inv_complete")).toBeNull();

    fireEvent.click(dialog.getByRole("button", { name: "Overview" }));
    expect(dialog.getByText("inv_complete")).toBeTruthy();
    expect(dialog.getByText("legacy:desktop")).toBeTruthy();
    fireEvent.click(dialog.getByRole("button", { name: "Governance" }));
    expect(dialog.getByText("policy.good")).toBeTruthy();
    expect(dialog.queryByText("provider_error")).toBeNull();
    expect(dialog.queryByText("Secret")).toBeNull();
  });

  it("opens Provider activation diagnostics only after selecting that history", async () => {
    mount([invocation], [activation]);
    await screen.findByRole("table", { name: "Invocation history" });
    fireEvent.click(screen.getByRole("button", { name: "Provider activation history" }));
    fireEvent.click(screen.getByRole("button", { name: "View Provider activation record" }));
    const dialog = within(screen.getByRole("dialog", { name: "Provider activation details" }));
    expect(dialog.getByText("Activation refused by policy")).toBeTruthy();
    expect(dialog.queryByText("provider_secure")).toBeNull();
    fireEvent.click(dialog.getByRole("button", { name: "Overview" }));
    expect(dialog.getByText("provider_secure")).toBeTruthy();
    expect(dialog.getByText("act_denied")).toBeTruthy();
    fireEvent.click(dialog.getByRole("button", { name: "Governance" }));
    expect(dialog.getByText("policy.blocked")).toBeTruthy();
  });

  it("refreshes both sources without creating another stacked table", async () => {
    const refreshed = { ...invocation, invocationId: "inv_new", toolId: "tool_new" };
    const listAudit = vi.fn()
      .mockResolvedValueOnce({ records: [invocation] })
      .mockResolvedValue({ records: [refreshed] });
    const { listProviderActivationAudit } = mount([invocation], [activation], { listAudit });
    expect(await screen.findByRole("table", { name: "Invocation history" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    await waitFor(() => expect(listAudit).toHaveBeenCalledTimes(2));
    expect(listProviderActivationAudit).toHaveBeenCalledTimes(2);
    expect(screen.getAllByRole("table")).toHaveLength(1);
  });
});
