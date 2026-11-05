import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient, Profile, SkillDeployment, SkillInstallation } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { ProfileSkillDeliveryPage } from "./profile-skill-delivery-page";

const installationId = "sinst_00000000000000000000000000000001";
const profile: Profile = {
  profileId: "prof_00000000000000000000000000000001",
  slug: "delivery", displayName: "Delivery",
  revision: 3, tools: [], skills: [installationId],
};
const skill: SkillInstallation = {
  skillId: "skill_00000000000000000000000000000001",
  installationId, name: "mender-tools", description: "Mender instructions",
  sourceKind: "builtin", sourceReference: "skills/mender-tools",
  digest: "sha256:" + "a".repeat(64), versionHint: "1.0",
  trusted: true, enabled: true, builtIn: true,
};
const owned: SkillDeployment = {
  deploymentId: "sdep_00000000000000000000000000000001",
  installationId,
  profileId: profile.profileId, profileRevision: 3,
  target: "agents-project", root: "/tmp/project",
  destination: "/tmp/project/.agents/skills/mender-tools",
  digest: skill.digest, deployedAt: "2026-10-23T01:00:00Z", drift: "current",
};

function show({
  deploy, ownedDeployments = [],
}: {
  deploy?: (slug: string, target: SkillDeployment["target"]) => Promise<SkillDeployment[] | null>;
  ownedDeployments?: SkillDeployment[];
} = {}) {
  const listProfileSkillDeployments = vi.fn()
    .mockResolvedValueOnce({ deployments: ownedDeployments })
    .mockResolvedValue({ deployments: deploy ? [owned] : ownedDeployments });
  const undeployProfileSkill = vi.fn().mockResolvedValue(undefined);
  const client = {
    getProfile: vi.fn().mockResolvedValue(profile),
    listSkills: vi.fn().mockResolvedValue({ installations: [skill] }),
    listProfileSkillDeployments,
    undeployProfileSkill,
  } as unknown as MenderClient;
  render(<MemoryRouter initialEntries={["/profiles/delivery/deliveries/skills"]}>
    <TooltipProvider>
      <Routes>
        <Route path="/profiles/:slug/deliveries/skills" element={<ProfileSkillDeliveryPage
          client={client} {...(deploy ? { deployProfileSkills: deploy } : {})} />} />
        <Route path="/profiles/:slug" element={<div>Profile detail</div>} />
      </Routes>
    </TooltipProvider>
  </MemoryRouter>);
  return { listProfileSkillDeployments, undeployProfileSkill };
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("does not deploy by default; requires an explicit local target", async () => {
  const deploy = vi.fn().mockResolvedValue([owned]);
  const { listProfileSkillDeployments } = show({ deploy });
  expect(await screen.findByText("mender-tools")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Deploy selected skills" })).toHaveProperty("disabled", true);
  expect(deploy).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Deployment target"), { target: { value: "agents-project" } });
  expect(screen.getByText(/native file chooser will ask for a project directory/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Deploy selected skills" }));
  await waitFor(() => expect(deploy).toHaveBeenCalledWith("delivery", "agents-project"));
  await waitFor(() => expect(listProfileSkillDeployments).toHaveBeenCalledTimes(2));
  expect(await screen.findByText(owned.destination)).toBeTruthy();
  expect(screen.getByText("Deployed 1 selected skills.")).toBeTruthy();
});

it("treats cancelled native file chooser as no deployment and no success", async () => {
  const deploy = vi.fn().mockResolvedValue(null);
  const { listProfileSkillDeployments } = show({ deploy });
  await screen.findByText("mender-tools");
  fireEvent.change(screen.getByLabelText("Deployment target"), { target: { value: "agents-project" } });
  fireEvent.click(screen.getByRole("button", { name: "Deploy selected skills" }));
  await waitFor(() => expect(deploy).toHaveBeenCalledTimes(1));
  expect(listProfileSkillDeployments).toHaveBeenCalledTimes(1);
  expect(screen.queryByText("Deployed 1 selected skills.")).toBeNull();
});

it("renders read-only remote deliveries and hides host filesystem writes", async () => {
  show({ ownedDeployments: [{ ...owned, profileRevision: 1 }] });
  expect(await screen.findByText(owned.destination)).toBeTruthy();
  expect(screen.getByText(/Profile changed since delivery/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Deploy selected skills" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Undeploy" })).toBeNull();
  expect(screen.getByText(/Remote management is read-only/)).toBeTruthy();
});

it("requires an in-app confirmation before undeploying an exact Profile record", async () => {
  const deploy = vi.fn();
  const { undeployProfileSkill } = show({ deploy, ownedDeployments: [owned] });
  expect(await screen.findByText(owned.destination)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Undeploy" }));
  const dialog = within(screen.getByRole("dialog", { name: "Remove Skill deployment?" }));
  expect(undeployProfileSkill).not.toHaveBeenCalled();
  fireEvent.click(dialog.getByRole("button", { name: "Cancel" }));
  expect(undeployProfileSkill).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Undeploy" }));
  fireEvent.click(within(screen.getByRole("dialog", { name: "Remove Skill deployment?" }))
    .getByRole("button", { name: "Undeploy" }));
  await waitFor(() => expect(undeployProfileSkill).toHaveBeenCalledWith("delivery", owned.deploymentId));
  expect(screen.queryByText(owned.destination)).toBeNull();
});

it("preserves deployment records and inputs after a failure", async () => {
  const deploy = vi.fn().mockRejectedValue(new Error("Cannot write target directory"));
  show({ deploy, ownedDeployments: [owned] });
  await screen.findByText(owned.destination);
  fireEvent.change(screen.getByLabelText("Deployment target"), { target: { value: "agents-user" } });
  fireEvent.click(screen.getByRole("button", { name: "Deploy selected skills" }));
  expect((await screen.findByRole("alert")).textContent).toContain("Cannot write target directory");
  expect(screen.getByText(owned.destination)).toBeTruthy();
  expect(screen.getByLabelText("Deployment target")).toHaveProperty("value", "agents-user");
});
