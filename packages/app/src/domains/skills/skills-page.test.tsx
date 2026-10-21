import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type {
  MenderClient,
  SkillDeployment,
  SkillInspection,
  SkillInstallation,
} from "@mender/sdk";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MenderApp } from "../../mender-app";

afterEach(cleanup);

describe("SkillsPage", () => {
  it("uses one-category-at-a-time Skill detail dialogs", async () => {
    const installation: SkillInstallation = {
      skillId: "skill_0123456789abcdef0123456789abcdef",
      installationId: "sinst_0123456789abcdef0123456789abcdef",
      name: "example-skill",
      description: "Example Skill",
      sourceKind: "local-directory",
      sourceReference: "/tmp/example-skill",
      digest: "sha256:" + "a".repeat(64),
      versionHint: "1.0.0",
      trusted: true,
      enabled: true,
      builtIn: false,
      installedAt: "2026-09-28T00:00:00Z",
    };
    const builtIn: SkillInstallation = {
      skillId: "skill_c3e37b8fb5b18a1c1f97cb2f8fb4cdb1",
      installationId: "sinst_11112222333344445555666677778888",
      name: "mender-tools",
      description: "Mender tools",
      sourceKind: "builtin",
      sourceReference: "skills/mender-tools",
      digest: "sha256:" + "b".repeat(64),
      versionHint: "",
      trusted: true,
      enabled: true,
      builtIn: true,
    };
    const deployment: SkillDeployment = {
      deploymentId: "sdep_0123456789abcdef0123456789abcdef",
      installationId: installation.installationId,
      target: "agents-project",
      root: "/workspace",
      destination: "/workspace/.agents/skills/example-skill",
      digest: installation.digest,
      deployedAt: "2026-09-28T01:00:00Z",
      drift: "current",
      currentDigest: installation.digest,
    };
    const inspection: SkillInspection = {
      installation,
      metadata: {
        name: "example-skill",
        description: "Example Skill",
        license: "Apache-2.0",
        compatibility: "Requires git",
        metadata: { version: "1.0.0" },
        allowedTools: "Bash(git:*)",
      },
      resources: [
        {
          path: "SKILL.md",
          kind: "manifest",
          size: 128,
          sha256: "sha256:" + "c".repeat(64),
          executable: false,
        },
      ],
      totalBytes: 128,
    };

    const listSkills = vi.fn().mockResolvedValue({
      installations: [builtIn, installation],
    });
    const listSkillDeployments = vi.fn().mockResolvedValue({
      deployments: [deployment],
    });
    const getSkillInstallation = vi.fn().mockResolvedValue(inspection);
    const untrustSkillInstallation = vi.fn().mockResolvedValue({
      installation: { ...installation, trusted: false, enabled: false },
    });
    const undeploySkill = vi.fn().mockResolvedValue(undefined);
    const installSkill = vi.fn().mockResolvedValue(installation);
    const deploySkill = vi.fn().mockResolvedValue(deployment);

    const client = {
      listSkills,
      listSkillDeployments,
      getSkillInstallation,
      untrustSkillInstallation,
      trustSkillInstallation: vi.fn(),
      enableSkillInstallation: vi.fn(),
      disableSkillInstallation: vi.fn(),
      deleteSkillInstallation: vi.fn(),
      undeploySkill,
    } as unknown as MenderClient;

    render(
      <MemoryRouter initialEntries={["/skills"]}>
        <MenderApp
          client={client}
          installSkill={installSkill}
          deploySkill={deploySkill}
        />
      </MemoryRouter>,
    );

    expect(
      await screen.findByRole("heading", { name: "Skills" }),
    ).toBeTruthy();
    expect(screen.getAllByText("mender-tools").length).toBeGreaterThan(0);
    const toolbar = document.querySelector('[data-slot="page-toolbar"]');
    const results = screen.getByRole("region", { name: "Skill Library" });
    expect(toolbar).not.toBeNull();
    expect(toolbar?.contains(screen.getByRole("button", { name: "Install Skill" }))).toBe(true);
    expect(toolbar?.contains(screen.getByRole("textbox", { name: "Search Skills" }))).toBe(true);
    expect(toolbar?.contains(screen.getByRole("button", { name: /All Skills/ }))).toBe(true);
    expect(results.contains(toolbar)).toBe(false);
    expect(results.querySelector('[data-slot="card-header"]')).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "View example-skill" }),
    );

    expect(
      await screen.findByRole("heading", { name: "Skill details" }),
    ).toBeTruthy();
    const detailsDialog = screen.getByRole("dialog", { name: "Skill details" });
    expect(detailsDialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(detailsDialog.className).toContain("overflow-hidden");
    expect(detailsDialog.querySelector('[data-slot="dialog-body"]')?.className).toContain("overflow-y-auto");
    expect(detailsDialog.querySelector('[data-slot="dialog-footer"]')).toBeTruthy();
    await waitFor(() => {
      expect(getSkillInstallation).toHaveBeenCalledWith(
        installation.installationId,
      );
    });
    expect(screen.getByText("Bash(git:*)")).toBeTruthy();
    expect(screen.queryByText("SKILL.md")).toBeNull();
    expect(
      screen.queryByText("/workspace/.agents/skills/example-skill"),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "View resources" }),
    );
    expect(
      screen.getByRole("heading", { name: "Skill resources" }),
    ).toBeTruthy();
    expect(screen.getByText("SKILL.md")).toBeTruthy();
    expect(
      screen.queryByText("/workspace/.agents/skills/example-skill"),
    ).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(
      screen.getByRole("heading", { name: "Skill details" }),
    ).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "View deployments" }),
    );
    expect(
      screen.getByRole("heading", { name: "Skill deployments" }),
    ).toBeTruthy();
    expect(
      screen.getByText("/workspace/.agents/skills/example-skill"),
    ).toBeTruthy();
    expect(screen.queryByText("SKILL.md")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Undeploy example-skill" }),
    );
    await waitFor(() => {
      expect(undeploySkill).toHaveBeenCalledWith(deployment.deploymentId);
    });

    fireEvent.click(
      screen.getAllByRole("button", { name: "Close" })[0]!,
    );

    fireEvent.click(screen.getByRole("button", { name: "Install Skill" }));
    await waitFor(() => {
      expect(installSkill).toHaveBeenCalledTimes(1);
    });
    fireEvent.click(
      screen.getAllByRole("button", { name: "Close" })[0]!,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Deploy example-skill" }),
    );
    expect(
      await screen.findByRole("heading", { name: "Deploy Skill" }),
    ).toBeTruthy();
    const deployDialog = screen.getByRole("dialog", { name: "Deploy Skill" });
    expect(deployDialog.querySelector('[data-slot="dialog-body"]')).not.toBeNull();
    expect(within(deployDialog).getByRole("button", { name: "Cancel" })).toBeTruthy();
    expect(within(deployDialog).queryAllByRole("button", { name: "Close" })).toHaveLength(1);
    const targetSelect = within(deployDialog).getByRole<HTMLSelectElement>("combobox", { name: "Skill deployment target" });
    const deployButton = within(deployDialog).getByRole<HTMLButtonElement>("button", { name: "Deploy" });
    expect(targetSelect.value).toBe("");
    expect(deployButton.disabled).toBe(true);
    expect(deploySkill).not.toHaveBeenCalled();
    fireEvent.change(targetSelect, { target: { value: "agents-project" } });
    expect(within(deployDialog).getByText("Project deployment: choose a project folder in the next step.")).toBeTruthy();
    expect(deployButton.disabled).toBe(false);
    fireEvent.click(deployButton);
    await waitFor(() => {
      expect(deploySkill).toHaveBeenCalledWith(
        installation.installationId,
        "agents-project",
      );
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Untrust example-skill" }),
    );
    await waitFor(() => {
      expect(untrustSkillInstallation).toHaveBeenCalledWith(
        installation.installationId,
      );
    });
  });
});
