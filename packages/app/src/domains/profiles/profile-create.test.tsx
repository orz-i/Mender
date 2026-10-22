import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, Profile } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfileCreatePage } from "./profile-create-page";

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
  const client = { createProfile, ...overrides } as unknown as MenderClient;
  render(<MemoryRouter initialEntries={["/profiles/new"]}>
    <TooltipProvider>
      <Routes>
        <Route path="/profiles/new" element={<ProfileCreatePage client={client} />} />
        <Route path="/profiles/:slug/edit" element={<div>Tool editor</div>} />
        <Route path="/profiles" element={<div>Profiles inventory</div>} />
      </Routes>
    </TooltipProvider>
  </MemoryRouter>);
  return { createProfile };
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
  resetLocaleForTesting();
  setLocale("en");
});
afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("Profile create page", () => {
  it("shows a complete full-page form, normalizes its slug and opens editing after creation", async () => {
    const { createProfile } = openCreate();
    expect(screen.queryByRole("dialog")).toBeNull();
    enterProfile(" My   Profile ");
    expect(screen.getByText("Result: my-profile")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create profile" }));
    await waitFor(() => expect(createProfile).toHaveBeenCalledWith({
      slug: "my-profile", displayName: "My Profile", mode: "hybrid",
    }));
    expect(await screen.findByText("Tool editor")).toBeTruthy();
  });

  it("disables invalid slug and overlong display name with visible guidance", () => {
    const { createProfile } = openCreate();
    enterProfile("My_Profile");
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Profile slug" })
      .getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText(/Use 1–63 characters/)).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create profile" }).disabled).toBe(true);
    enterProfile("My Profile", "x".repeat(161));
    expect(screen.getByText("Result: my-profile")).toBeTruthy();
    expect(screen.getByText("Display name is too long (maximum 160 bytes).")).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create profile" }).disabled).toBe(true);
    expect(createProfile).not.toHaveBeenCalled();
  });

  it("surfaces duplicate canonical identities on the page and allows retry", async () => {
    const createProfile = vi.fn()
      .mockRejectedValueOnce(new Error("create profile: profile: slug already exists"))
      .mockResolvedValueOnce(result);
    openCreate({ createProfile });
    enterProfile("My Profile");
    fireEvent.click(screen.getByRole("button", { name: "Create profile" }));
    expect((await screen.findByRole("alert")).textContent).toContain("This Profile slug is already in use.");
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Profile slug" }).value).toBe("My Profile");
    fireEvent.change(screen.getByRole("textbox", { name: "Profile slug" }),
      { target: { value: "My Profile 2" } });
    expect(screen.queryByRole("alert")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Create profile" }));
    await waitFor(() => expect(createProfile).toHaveBeenLastCalledWith({
      slug: "my-profile-2", displayName: "My Profile", mode: "hybrid",
    }));
  });

  it("localizes validation feedback without altering display name case", async () => {
    resetLocaleForTesting();
    setLocale("zh-CN");
    const { createProfile } = openCreate();
    fireEvent.change(screen.getByRole("textbox", { name: "配置档 slug" }),
      { target: { value: "My Profile" } });
    fireEvent.change(screen.getByRole("textbox", { name: "配置档显示名称" }),
      { target: { value: "My Profile" } });
    expect(screen.getByText("规范标识：my-profile")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "创建配置档" }));
    await waitFor(() => expect(createProfile).toHaveBeenCalledWith(expect.objectContaining({
      slug: "my-profile", displayName: "My Profile",
    })));
  });

  it("lets users cancel without persisting", () => {
    const { createProfile } = openCreate();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("Profiles inventory")).toBeTruthy();
    expect(createProfile).not.toHaveBeenCalled();
  });
});
