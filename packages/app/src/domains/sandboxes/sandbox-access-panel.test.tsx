import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient, Profile, Sandbox, SandboxProfileGrants } from "@mender/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { SandboxAccessPanel } from "./sandbox-access-panel";

const sandbox: Sandbox = {
  sandboxId: "sbx_0123456789abcdef0123456789abcdef",
  name: "Sample workspace", backend: "local_workspace", isolation: "host", state: "ready",
  localRoot: "/tmp/sample", ownerKind: "local_user", ownerRef: "os:501",
  activeLeases: 0, backendAvailable: true,
};
const profile: Profile = {
  profileId: "prof_0123456789abcdef0123456789abcdef",
  slug: "demo", displayName: "Demo MCP Profile", mode: "hybrid", tools: [],
};
const empty: SandboxProfileGrants = { sandboxId: sandbox.sandboxId, grants: [] };
const granted: SandboxProfileGrants = {
  sandboxId: sandbox.sandboxId,
  grants: [{ profileId: profile.profileId, capabilities: ["file.read", "file.search"] }],
};

function mountAccess(overrides: Partial<MenderClient> = {}, sandboxValue = sandbox) {
  const put = vi.fn().mockResolvedValue(granted);
  const revoke = vi.fn().mockResolvedValue(empty);
  const client = {
    getSandboxProfileGrants: vi.fn().mockResolvedValue(empty),
    getSandboxProfileGrantEvents: vi.fn().mockResolvedValue({
      sandboxId: sandbox.sandboxId,
      events: [{
        profileId: profile.profileId, action: "grant", capability: "file.read",
        occurredAt: "2026-10-11T01:00:00Z",
      }],
    }),
    listProfiles: vi.fn().mockResolvedValue({ profiles: [profile] }),
    putSandboxProfileGrants: put,
    revokeSandboxProfileGrants: revoke,
    ...overrides,
  } as MenderClient;
  render(<SandboxAccessPanel client={client} sandbox={sandboxValue} />);
  return { put, revoke };
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
});

describe("owner-managed Sandbox Profile access", () => {
  it("defaults to no access and requires selection, review and confirmation before granting", async () => {
    const nativeConfirm = vi.spyOn(window, "confirm").mockImplementation(() => {
      throw new Error("WKWebView confirm unsupported");
    });
    const { put } = mountAccess();
    expect(await screen.findByText(/No Profile access granted/)).toBeTruthy();
    expect(put).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Grant Profile access" }));

    const form = within(screen.getByRole("dialog", { name: "Configure Profile access" }));
    expect(form.getByRole<HTMLSelectElement>("combobox", { name: "Choose Profile for access" }).value).toBe(profile.profileId);
    expect(form.getByRole<HTMLInputElement>("checkbox", { name: "Read files" }).checked).toBe(true);
    expect(form.getByRole<HTMLInputElement>("checkbox", { name: "Search files" }).checked).toBe(true);
    expect(form.getByRole<HTMLInputElement>("checkbox", { name: "Edit files" }).checked).toBe(false);
    expect(form.getByRole<HTMLInputElement>("checkbox", { name: "Run shell" }).checked).toBe(false);
    fireEvent.click(form.getByRole("button", { name: "Review access" }));

    const review = within(screen.getByRole("dialog", { name: "Review Profile access" }));
    expect(review.getByText(/Every remote caller of this Profile/)).toBeTruthy();
    expect(put).not.toHaveBeenCalled();
    fireEvent.click(review.getByRole("button", { name: "Confirm access" }));
    await waitFor(() => expect(put).toHaveBeenCalledExactlyOnceWith(
      sandbox.sandboxId, profile.profileId, { capabilities: ["file.read", "file.search"] },
    ));
    expect(await screen.findByText("Demo MCP Profile")).toBeTruthy();
    expect(nativeConfirm).not.toHaveBeenCalled();
  });

  it("disables empty grant requests; host shell warns about OS user authority", async () => {
    const { put } = mountAccess();
    await screen.findByText(/No Profile access granted/);
    fireEvent.click(screen.getByRole("button", { name: "Grant Profile access" }));

    fireEvent.click(screen.getByRole("checkbox", { name: "Read files" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Search files" }));
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Review access" }).disabled).toBe(true);
    fireEvent.click(screen.getByRole("checkbox", { name: "Run shell" }));
    expect(screen.getByText(/Host shell is not isolated/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(put).not.toHaveBeenCalled();
  });

  it("revoke requires separate confirmation and writes no grants after approval", async () => {
    const { revoke } = mountAccess({
      getSandboxProfileGrants: vi.fn().mockResolvedValue(granted),
    });
    expect(await screen.findByText("Demo MCP Profile")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Revoke access" }));
    const dialog = within(screen.getByRole("dialog", { name: "Revoke Profile access" }));
    expect(dialog.getByText(/Revoke all new Sandbox execution access/)).toBeTruthy();
    expect(revoke).not.toHaveBeenCalled();
    fireEvent.click(dialog.getByRole("button", { name: "Cancel" }));
    expect(revoke).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Revoke access" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm revoke" }));
    await waitFor(() => expect(revoke).toHaveBeenCalledExactlyOnceWith(
      sandbox.sandboxId, profile.profileId,
    ));
    expect(await screen.findByText(/No Profile access granted/)).toBeTruthy();
  });

  it("keeps an API error in the confirmation dialog and exposes audit separately", async () => {
    const put = vi.fn().mockRejectedValue(new Error("simulated conflict"));
    mountAccess({ putSandboxProfileGrants: put });
    await screen.findByText(/No Profile access granted/);
    fireEvent.click(screen.getByRole("button", { name: "Grant Profile access" }));
    fireEvent.click(screen.getByRole("button", { name: "Review access" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm access" }));
    expect(await within(screen.getByRole("dialog")).findByText("simulated conflict")).toBeTruthy();
    expect(screen.getByRole("dialog", { name: "Review Profile access" })).toBeTruthy();
    expect(put).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Access history" }));
    expect(await screen.findByText(/Granted/)).toBeTruthy();
    expect(screen.queryByText(/No Profile access granted/)).toBeNull();
  });

  it("does not enable mutations for a destroyed Sandbox", async () => {
    mountAccess({}, { ...sandbox, state: "destroyed" });
    await screen.findByText(/No Profile access granted/);
    expect(screen.queryByRole("button", { name: "Grant Profile access" })).toBeNull();
  });
});
