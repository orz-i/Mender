import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient, Profile } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfilesPage } from "./profiles-page";

const result: Profile = {
  profileId: "prof_0123456789abcdef0123456789abcdef",
  slug: "my-profile",
  displayName: "My Profile",
  mode: "hybrid",
  tools: [],
  tunnelId: "",
};

function openCreate(overrides: Partial<MenderClient> = {}) {
  const createProfile = vi.fn().mockResolvedValue(result);
  const listProfiles = vi.fn().mockResolvedValue({ profiles: [] });
  const client = {
    listProfiles,
    listProviders: vi.fn().mockResolvedValue({ providers: [] }),
    listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
    listTunnels: vi.fn().mockResolvedValue({ tunnels: [] }),
    browseTools: vi.fn().mockResolvedValue({ tools: [] }),
    createProfile,
    ...overrides,
  } as unknown as MenderClient;
  render(<TooltipProvider><ProfilesPage client={client} /></TooltipProvider>);
  fireEvent.click(screen.getByRole("button", { name: "Create profile" }));
  return { createProfile, listProfiles };
}

function enterProfile(slug: string, displayName = "My Profile") {
  fireEvent.change(screen.getByRole("textbox", { name: "Profile slug" }), {
    target: { value: slug },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "Profile display name" }), {
    target: { value: displayName },
  });
}

beforeEach(() => {
  window.localStorage.clear();
  resetLocaleForTesting();
  setLocale("en");
});
afterEach(() => {
  cleanup();
  resetLocaleForTesting();
  window.localStorage.clear();
});

describe("Profile create identifier UX", () => {
  it("previews and sends the canonical slug for mixed-case human input", async () => {
    const { createProfile } = openCreate();
    enterProfile(" My   Profile ");

    const dialog = within(screen.getByRole("dialog", { name: "Create profile" }));
    expect(dialog.getByText("Result: my-profile")).toBeTruthy();
    fireEvent.click(dialog.getByRole("button", { name: "Create profile" }));

    await waitFor(() => expect(createProfile).toHaveBeenCalledWith({
      slug: "my-profile",
      displayName: "My Profile",
      mode: "hybrid",
    }));
  });

  it("disables invalid slug and overlong display name with visible guidance", () => {
    const { createProfile } = openCreate();
    const dialog = within(screen.getByRole("dialog", { name: "Create profile" }));
    enterProfile("My_Profile");
    expect(dialog.getByRole<HTMLInputElement>("textbox", { name: "Profile slug" }).getAttribute("aria-invalid")).toBe("true");
    expect(dialog.getByText(/Use 1–63 characters/)).toBeTruthy();
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Create profile" }).disabled).toBe(true);

    enterProfile("My Profile", "x".repeat(161));
    expect(dialog.getByText("Result: my-profile")).toBeTruthy();
    expect(dialog.getByText("Display name is too long (maximum 160 bytes).")).toBeTruthy();
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Create profile" }).disabled).toBe(true);
    expect(createProfile).not.toHaveBeenCalled();
  });

  it("surfaces duplicate canonical identities inside the dialog and allows retry", async () => {
    const createProfile = vi.fn()
      .mockRejectedValueOnce(new Error("create profile: profile: slug already exists"))
      .mockResolvedValueOnce(result);
    openCreate({ createProfile });
    enterProfile("My Profile");
    const dialog = within(screen.getByRole("dialog", { name: "Create profile" }));
    fireEvent.click(dialog.getByRole("button", { name: "Create profile" }));

    expect((await dialog.findByRole("alert")).textContent).toContain("This Profile slug is already in use.");
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(dialog.getByRole<HTMLInputElement>("textbox", { name: "Profile slug" }).value).toBe("My Profile");
    fireEvent.change(dialog.getByRole("textbox", { name: "Profile slug" }), { target: { value: "My Profile 2" } });
    expect(dialog.queryByRole("alert")).toBeNull();
    fireEvent.click(dialog.getByRole("button", { name: "Create profile" }));
    await waitFor(() => expect(createProfile).toHaveBeenLastCalledWith({
      slug: "my-profile-2",
      displayName: "My Profile",
      mode: "hybrid",
    }));
  });

  it("localizes invalid-input feedback without changing display name case", () => {
    cleanup();
    resetLocaleForTesting();
    setLocale("zh-CN");

    const createProfile = vi.fn().mockResolvedValue(result);
    const client = {
      listProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
      listProviders: vi.fn().mockResolvedValue({ providers: [] }),
      listCredentials: vi.fn().mockResolvedValue({ credentials: [] }),
      listTunnels: vi.fn().mockResolvedValue({ tunnels: [] }),
      browseTools: vi.fn().mockResolvedValue({ tools: [] }),
      createProfile,
    } as unknown as MenderClient;
    render(<TooltipProvider><ProfilesPage client={client} /></TooltipProvider>);
    fireEvent.click(screen.getByRole("button", { name: "创建配置档" }));
    fireEvent.change(screen.getByRole("textbox", { name: "配置档 slug" }), {
      target: { value: "My Profile" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "配置档显示名称" }), {
      target: { value: "My Profile" },
    });
    expect(screen.getByText("规范标识：my-profile")).toBeTruthy();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "创建配置档" }));
    expect(createProfile).toHaveBeenCalledWith(expect.objectContaining({
      slug: "my-profile",
      displayName: "My Profile",
    }));
  });
});
