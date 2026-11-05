import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ManagementClient, Profile, SkillInstallation } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfileSkillsPage } from "./profile-skills-page";

const builtInID = "sinst_00000000000000000000000000000001";
const oldID = "sinst_00000000000000000000000000000002";
const profile: Profile = {
  profileId: "prof_00000000000000000000000000000001",
  slug: "delivery", displayName: "Delivery",
  revision: 0, tools: [], skills: [oldID],
};

function installation(id: string, name: string, enabled: boolean): SkillInstallation {
  return {
    skillId: "skill_00000000000000000000000000000001",
    installationId: id, name,
    description: "Instructions for " + name,
    sourceKind: "builtin", sourceReference: "skills/" + name,
    digest: "sha256:" + "a".repeat(64), versionHint: "1.0.0",
    trusted: true, enabled, builtIn: true,
  };
}

function show(options: { replace?: ManagementClient["replaceProfileSkills"]; selected?: string[] } = {}) {
  const replaceProfileSkills = options.replace ?? vi.fn().mockResolvedValue(profile);
  const listSkills = vi.fn().mockResolvedValue({
    installations: [
      installation(builtInID, "ready-skill", true),
      installation(oldID, "old-skill", false),
      installation("sinst_00000000000000000000000000000003", "disabled", false),
    ],
  });
  const client = {
    getProfile: vi.fn().mockResolvedValue({ ...profile, skills: options.selected ?? [oldID] }),
    listSkills,
    replaceProfileSkills,
  } as unknown as ManagementClient;
  render(<MemoryRouter initialEntries={["/profiles/delivery/skills"]}>
    <TooltipProvider><Routes>
      <Route path="/profiles/:slug/skills" element={<ProfileSkillsPage client={client} />} />
      <Route path="/profiles/:slug" element={<span>Profile detail shown</span>} />
    </Routes></TooltipProvider>
  </MemoryRouter>);
  return { replaceProfileSkills, listSkills };
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("requires explicit selection and rejects disabled additions while preserving removal", async () => {
  const { replaceProfileSkills } = show();
  expect((await screen.findByRole("checkbox", { name: "Select skill old-skill" })).getAttribute("type")).toBe("checkbox");
  expect(screen.getByRole("checkbox", { name: "Select skill old-skill" })).toHaveProperty("checked", true);
  expect(screen.queryByRole("checkbox", { name: "Select skill disabled" })).toBeNull();
  expect(screen.getByRole("button", { name: "Save skills" })).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("checkbox", { name: "Select skill old-skill" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "Select skill ready-skill" }));
  fireEvent.click(screen.getByRole("button", { name: "Save skills" }));
  await waitFor(() => expect(replaceProfileSkills).toHaveBeenCalledWith("delivery", {
    installationIds: [builtInID],
  }));
  expect(await screen.findByText("Profile detail shown")).toBeTruthy();
});

it("preserves the selection and shows an inline failure without any file deployment", async () => {
  const replace = vi.fn().mockRejectedValue(new Error("Profile revision changed"));
  show({ replace, selected: [] });
  fireEvent.click(await screen.findByRole("checkbox", { name: "Select skill ready-skill" }));
  fireEvent.click(screen.getByRole("button", { name: "Save skills" }));
  expect((await screen.findByRole("alert")).textContent).toContain("Profile revision changed");
  expect(screen.getByRole("checkbox", { name: "Select skill ready-skill" })).toHaveProperty("checked", true);
  expect(replace).toHaveBeenCalledTimes(1);
});

it("does not drop an unknown historical installation automatically", async () => {
  const unknownID = "sinst_00000000000000000000000000000009";
  const { replaceProfileSkills } = show({ selected: [unknownID] });
  const unknown = await screen.findByRole("checkbox", { name: "Remove unavailable skill " + unknownID });
  expect(unknown).toHaveProperty("checked", true);
  expect(screen.getByRole("button", { name: "Save skills" })).toHaveProperty("disabled", true);
  fireEvent.click(unknown);
  fireEvent.click(screen.getByRole("button", { name: "Save skills" }));
  await waitFor(() => expect(replaceProfileSkills).toHaveBeenCalledWith("delivery", { installationIds: [] }));
});

it("cancels without mutating any Profile or Skill files", async () => {
  const { replaceProfileSkills } = show({ selected: [] });
  fireEvent.click(await screen.findByRole("checkbox", { name: "Select skill ready-skill" }));
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(await screen.findByText("Profile detail shown")).toBeTruthy();
  expect(replaceProfileSkills).not.toHaveBeenCalled();
});
