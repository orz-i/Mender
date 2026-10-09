import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, SandboxActivity } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SandboxActivityPage } from "./sandbox-activity-page";

afterEach(cleanup);

const id = "sbx_0123456789abcdef0123456789abcdef";
const activity: SandboxActivity = {
  sandboxId: id,
  limit: 100,
  events: [
    { kind: "lifecycle", action: "sandbox.destroy", status: "completed", occurredAt: "2026-10-09T12:00:00Z", principalKind: "local_user", origin: "management_http", generation: 2, previousState: "ready", nextState: "destroyed" },
    { kind: "admission", action: "sandbox.stop", status: "denied", occurredAt: "2026-10-09T11:00:00Z", principalKind: "local_user", origin: "management_http", policyDecision: "deny" },
  ],
};

function mount(client: Partial<MenderClient>) {
  render(<MemoryRouter initialEntries={[`/sandboxes/${id}/activity`]}>
    <Routes>
      <Route path="/sandboxes/:sandboxId/activity" element={<SandboxActivityPage client={client as MenderClient} />} />
      <Route path="/sandboxes/:sandboxId" element={<div>Sandbox overview reached</div>} />
    </Routes>
  </MemoryRouter>);
}

describe("Sandbox activity", () => {
  it("shows denial and successful lifecycle after deletion, with no private payloads", async () => {
    const getSandboxActivity = vi.fn().mockResolvedValue(activity);
    mount({ getSandboxActivity });
    expect(await screen.findByText("sandbox.destroy")).toBeTruthy();
    expect(screen.getByText("sandbox.stop")).toBeTruthy();
    expect(screen.getByText("ready → destroyed", { exact: false })).toBeTruthy();
    expect(getSandboxActivity).toHaveBeenCalledWith(id, 100);
    expect(document.body.textContent).not.toContain("credential://");
    expect(document.body.textContent).not.toContain("secret-principal");
    expect(document.body.textContent).not.toContain("policy reason");
    fireEvent.change(screen.getByRole("combobox", { name: "Event type" }), { target: { value: "admission" } });
    expect(screen.queryByText("sandbox.destroy")).toBeNull();
    expect(screen.getByText("sandbox.stop")).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: "Event type" }), { target: { value: "lifecycle" } });
    expect(screen.queryByText("sandbox.stop")).toBeNull();
    expect(screen.getByText("sandbox.destroy")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Back to sandbox" }));
    expect(await screen.findByText("Sandbox overview reached")).toBeTruthy();
  });

  it("shows empty state without manufacturing events", async () => {
    mount({ getSandboxActivity: vi.fn().mockResolvedValue({ sandboxId: id, limit: 100, events: [] }) });
    expect(await screen.findByText("No activity yet")).toBeTruthy();
    expect(screen.queryByText("sandbox.acquire")).toBeNull();
  });

  it("shows a fetch error rather than an empty successful audit", async () => {
    mount({ getSandboxActivity: vi.fn().mockRejectedValue(new Error("Activity service unavailable")) });
    await waitFor(() => expect(screen.getByText("Activity service unavailable")).toBeTruthy());
    expect(screen.queryByText("No activity yet")).toBeNull();
  });
});
