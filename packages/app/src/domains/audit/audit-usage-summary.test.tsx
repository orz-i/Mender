import { cleanup, render, screen, within } from "@testing-library/react";
import type { AuditRecord } from "@mender/sdk";
import { afterEach, expect, it } from "vitest";

import { AuditUsageSummary } from "./audit-usage-summary";

afterEach(cleanup);

it("graphs only observed invocation history with explicit sample boundaries", () => {
  const records = [
    { invocationId: "r1", toolId: "tool_a", status: "completed", finishedAt: "2026-11-02T12:00:00Z" },
    { invocationId: "r2", toolId: "tool_a", status: "completed", finishedAt: "2026-11-04T08:00:00Z" },
    { invocationId: "r3", toolId: "tool_b", status: "failed", finishedAt: "2026-11-04T11:00:00Z" },
  ] as AuditRecord[];
  render(<AuditUsageSummary records={records} />);
  const summary = screen.getByRole("region", { name: "Recent invocation sample" });
  expect(within(summary).getByText("Based on up to 100 latest records, not complete historical totals.")).toBeTruthy();
  expect(within(summary).getByRole("img", { name: "Recorded invocations by day in the latest sample" })).toBeTruthy();
  const bars = summary.querySelectorAll('[role="img"] [title]');
  expect(bars).toHaveLength(2);
  expect([...bars].some((bar) => bar.getAttribute("title")?.endsWith(": 2"))).toBe(true);
  expect(within(summary).getByText("Distinct tools")).toBeTruthy();
});

it("does not fabricate a chart or summary when no audit records exist", () => {
  render(<AuditUsageSummary records={[]} />);
  expect(screen.queryByRole("region", { name: "Recent invocation sample" })).toBeNull();
});
