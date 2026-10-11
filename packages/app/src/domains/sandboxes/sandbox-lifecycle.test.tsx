import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient, Sandbox } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { SandboxDetailPage, SandboxesPage } from "./sandboxes-page";

const local: Sandbox = {
  sandboxId: "sbx_0123456789abcdef0123456789abcdef",
  name: "Test Sandbox",
  backend: "local_workspace",
  isolation: "host",
  state: "ready",
  localRoot: "/tmp/test-sandbox",
  ownerKind: "local_user",
  ownerRef: "owner",
  activeLeases: 0,
  backendAvailable: true,
};

function mount(client: Partial<MenderClient>) {
  render(
    <MemoryRouter initialEntries={[`/sandboxes/${local.sandboxId}`]}>
      <Routes>
        <Route path="/sandboxes/:sandboxId" element={<SandboxDetailPage client={client as MenderClient} />} />
      </Routes>
    </MemoryRouter>,
  );
}

async function openManage() {
  fireEvent.click(await screen.findByRole("button", { name: "Manage sandbox" }));
  return within(screen.getByRole("dialog", { name: "Manage sandbox" }));
}

async function requestDestroy() {
  const manage = await openManage();
  fireEvent.click(manage.getByRole("button", { name: "Destroy sandbox" }));
  return within(screen.getByRole("dialog", { name: "Confirm sandbox destruction" }));
}

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
  window.localStorage.clear();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  resetLocaleForTesting();
  window.localStorage.clear();
});

describe("Sandbox lifecycle management", () => {
  it("offers Start/Stop only for ready/stopped resources and updates state", async () => {
    const stopSandbox = vi.fn().mockResolvedValue({ ...local, state: "stopped" });
    const startSandbox = vi.fn().mockResolvedValue({ ...local, state: "ready" });
    mount({ getSandbox: vi.fn().mockResolvedValue(local), stopSandbox, startSandbox });

    const manage = await openManage();
    fireEvent.click(manage.getByRole("button", { name: "Stop sandbox" }));
    await waitFor(() => expect(stopSandbox).toHaveBeenCalledWith(local.sandboxId));
    fireEvent.click(await manage.findByRole("button", { name: "Start sandbox" }));
    await waitFor(() => expect(startSandbox).toHaveBeenCalledWith(local.sandboxId));
    expect(await manage.findByRole("button", { name: "Stop sandbox" })).toBeTruthy();
  });

  it("blocks all lifecycle actions while a persisted execution lease is active", async () => {
    const stopSandbox = vi.fn();
    const destroySandbox = vi.fn();
    mount({ getSandbox: vi.fn().mockResolvedValue({ ...local, activeLeases: 1 }), stopSandbox, destroySandbox });
    const manage = await openManage();
    const stop = manage.getByRole<HTMLButtonElement>("button", { name: "Stop sandbox" });
    expect(stop.disabled).toBe(true);
    expect(manage.getByRole<HTMLButtonElement>("button", { name: "Destroy sandbox" }).disabled).toBe(true);
    expect(screen.getAllByText("Cannot change lifecycle while a Sandbox execution lease is active.").length).toBeGreaterThanOrEqual(1);
    expect(stopSandbox).not.toHaveBeenCalled();
    expect(destroySandbox).not.toHaveBeenCalled();
  });

  it("confirms destruction inside the same WebView dialog and never calls native window.confirm", async () => {
    const oldConfirm = vi.spyOn(window, "confirm").mockImplementation(() => {
      throw new Error("WKWebView does not implement window.confirm");
    });
    const destroySandbox = vi.fn().mockResolvedValue({ ...local, state: "destroyed" });
    mount({ getSandbox: vi.fn().mockResolvedValue(local), destroySandbox });

    const confirmation = await requestDestroy();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(confirmation.getByText(/only tombstones the Sandbox/)).toBeTruthy();
    expect(confirmation.getByText(/remains in inventory as a destroyed record/)).toBeTruthy();
    expect(destroySandbox).not.toHaveBeenCalled();

    fireEvent.click(confirmation.getByRole("button", { name: "Back" }));
    const manage = within(screen.getByRole("dialog", { name: "Manage sandbox" }));
    expect(manage.getByRole("button", { name: "Destroy sandbox" })).toBeTruthy();
    expect(destroySandbox).not.toHaveBeenCalled();

    fireEvent.click(manage.getByRole("button", { name: "Destroy sandbox" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Confirm sandbox destruction" }))
      .getByRole("button", { name: "Confirm destruction" }));

    await waitFor(() => expect(destroySandbox).toHaveBeenCalledExactlyOnceWith(local.sandboxId));
    expect(oldConfirm).not.toHaveBeenCalled();
    expect(await screen.findByText("Host files remain untouched after Sandbox destruction.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Manage sandbox" })).toBeNull();
  });

  it("closing the confirmation leaves the Sandbox unchanged and does not call the backend", async () => {
    const destroySandbox = vi.fn();
    mount({ getSandbox: vi.fn().mockResolvedValue(local), destroySandbox });

    await requestDestroy();
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    const footer = screen.getByRole("dialog", { name: "Manage sandbox" })
      .querySelector('[data-slot="dialog-footer"]');
    expect(footer).toBeTruthy();
    fireEvent.click(within(footer as HTMLElement).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("Test Sandbox")).toBeTruthy();
    expect(destroySandbox).not.toHaveBeenCalled();
  });

  it("shows an actionable conflict in the same dialog and permits retry without silent success", async () => {
    const destroySandbox = vi.fn()
      .mockRejectedValueOnce(new Error("sandbox state conflict"))
      .mockResolvedValueOnce({ ...local, state: "destroyed" });
    mount({ getSandbox: vi.fn().mockResolvedValue(local), destroySandbox });

    const confirmation = await requestDestroy();
    fireEvent.click(confirmation.getByRole("button", { name: "Confirm destruction" }));
    expect((await confirmation.findByText(/Sandbox lifecycle conflicts with an active lease/))).toBeTruthy();
    expect(screen.getByRole("dialog", { name: "Confirm sandbox destruction" })).toBeTruthy();
    expect(destroySandbox).toHaveBeenCalledTimes(1);

    fireEvent.click(confirmation.getByRole("button", { name: "Confirm destruction" }));
    await waitFor(() => expect(destroySandbox).toHaveBeenCalledTimes(2));
    expect(await screen.findByText("Host files remain untouched after Sandbox destruction.")).toBeTruthy();
  });

  it("uses the same in-dialog two-step flow for destructive Docker restart", async () => {
    const docker: Sandbox = {
      ...local,
      backend: "docker",
      isolation: "container",
      state: "stopped",
      localRoot: "",
    };
    const startSandbox = vi.fn().mockResolvedValue({ ...docker, state: "ready" });
    const oldConfirm = vi.spyOn(window, "confirm").mockImplementation(() => {
      throw new Error("WKWebView confirm unavailable");
    });
    mount({ getSandbox: vi.fn().mockResolvedValue(docker), startSandbox });

    const manage = await openManage();
    fireEvent.click(manage.getByRole("button", { name: "Start sandbox" }));
    const confirmation = within(screen.getByRole("dialog", { name: "Confirm Docker sandbox restart" }));
    expect(confirmation.getByText(/workspace will be empty/)).toBeTruthy();
    expect(startSandbox).not.toHaveBeenCalled();
    fireEvent.click(confirmation.getByRole("button", { name: "Back" }));
    expect(startSandbox).not.toHaveBeenCalled();
    fireEvent.click(manage.getByRole("button", { name: "Start sandbox" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm restart" }));
    await waitFor(() => expect(startSandbox).toHaveBeenCalledExactlyOnceWith(docker.sandboxId));
    expect(oldConfirm).not.toHaveBeenCalled();
  });

  it("keeps security details separate from identity preview", async () => {
    mount({ getSandbox: vi.fn().mockResolvedValue(local) });
    expect(await screen.findByText("Test Sandbox")).toBeTruthy();
    expect(screen.getByText(local.sandboxId)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Stop sandbox" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Security boundary" }));
    expect(screen.getByText("Host mode is not isolated and uses the Runtime user's local file and process permissions.")).toBeTruthy();
    expect(screen.queryByText(local.sandboxId)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Sandbox overview" }));
    expect(screen.getByText(local.sandboxId)).toBeTruthy();
  });

  it("separates Profile access from Sandbox overview", async () => {
    mount({
      getSandbox: vi.fn().mockResolvedValue(local),
      getSandboxProfileGrants: vi.fn().mockResolvedValue({ sandboxId: local.sandboxId, grants: [] }),
      getSandboxProfileGrantEvents: vi.fn().mockResolvedValue({ sandboxId: local.sandboxId, events: [] }),
      listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
    });
    expect(await screen.findByText(local.sandboxId)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Profile access" }));
    expect(await screen.findByText(/No Profile access granted/)).toBeTruthy();
    expect(screen.queryByText(local.sandboxId)).toBeNull();
  });

  it("does not display unsupported lifecycle actions on unregistered backends", async () => {
    mount({ getSandbox: vi.fn().mockResolvedValue({ ...local, backend: "e2b", isolation: "remote" }) });
    expect(await screen.findByText("Test Sandbox")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Start sandbox" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Stop sandbox" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Destroy sandbox" })).toBeNull();
  });

  it("displays destroyed tombstones in the list with localized status instead of backend unavailable", async () => {
    setLocale("zh-CN");
    render(
      <MemoryRouter>
        <SandboxesPage client={{
          listSandboxes: vi.fn().mockResolvedValue({
            sandboxes: [{ ...local, state: "destroyed", backendAvailable: false }],
          }),
        } as unknown as MenderClient} />
      </MemoryRouter>,
    );
    expect(await screen.findByText("已销毁")).toBeTruthy();
    expect(screen.queryByText("后端不可用")).toBeNull();
  });
});
