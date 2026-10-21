import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { MenderClient, SkillDeployment, SkillInstallation } from "@mender/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { SkillsPage } from "./skills-page";

const installations: SkillInstallation[] = [
  {
    skillId: "skill_0123456789abcdef0123456789abcdef",
    installationId: "sinst_0123456789abcdef0123456789abcdef",
    name: "workspace-review",
    description: "Review workspace changes",
    sourceKind: "local-directory",
    sourceReference: "/tmp/workspace-review",
    digest: "sha256:" + "a".repeat(64),
    versionHint: "1.1.0",
    trusted: true,
    enabled: true,
    builtIn: false,
    installedAt: "2026-10-21T00:00:00Z",
  },
  {
    skillId: "skill_1123456789abcdef0123456789abcdef",
    installationId: "sinst_1123456789abcdef0123456789abcdef",
    name: "analyze-logs",
    description: "Analyze local logs",
    sourceKind: "local-directory",
    sourceReference: "/tmp/analyze-logs",
    digest: "sha256:" + "b".repeat(64),
    versionHint: "2.0.0",
    trusted: true,
    enabled: true,
    builtIn: false,
    installedAt: "2026-10-21T00:00:00Z",
  },
];

const deployed: SkillDeployment = {
  deploymentId: "sdep_0123456789abcdef0123456789abcdef",
  installationId: installations[0]!.installationId,
  target: "agents-user",
  root: "/tmp/skills-user",
  destination: "/tmp/skills-user/.agents/skills/workspace-review",
  digest: installations[0]!.digest,
  deployedAt: "2026-10-21T00:00:00Z",
  drift: "current",
};

function showSkills(
  deploySkill: (installationId: string, target: SkillDeployment["target"]) => Promise<SkillDeployment | null>,
) {
  const client = {
    listSkills: vi.fn().mockResolvedValue({ installations }),
    listSkillDeployments: vi.fn().mockResolvedValue({ deployments: [] }),
  } as unknown as MenderClient;
  render(<SkillsPage client={client} deploySkill={deploySkill} />);
}

async function openDeployment(name: string) {
  fireEvent.click(await screen.findByRole("button", { name: "Deploy " + name }));
  return screen.getByRole("dialog", { name: "Deploy Skill" });
}

beforeEach(() => {
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
});

describe("Skill deployment form safety", () => {
  it("requires choosing the destination every time and distinguishes project versus user scope", async () => {
    const deploy = vi.fn().mockResolvedValue(deployed);
    showSkills(deploy);
    const first = await openDeployment("workspace-review");
    const target = within(first).getByRole<HTMLSelectElement>("combobox", { name: "Skill deployment target" });
    const save = within(first).getByRole<HTMLButtonElement>("button", { name: "Deploy" });
    expect(first.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(first.querySelector('[data-slot="dialog-body"]')).not.toBeNull();
    expect(target.value).toBe("");
    expect(save.disabled).toBe(true);
    expect(within(first).getByText("Choose a deployment target")).toBeTruthy();
    expect(within(first).getByText("workspace-review")).toBeTruthy();
    expect(within(first).getByText("Version 1.1.0 · Local")).toBeTruthy();
    expect(within(first).getByText("Installation ID")).toBeTruthy();
    expect(within(first).queryAllByRole("button", { name: "Close" })).toHaveLength(1);
    expect(deploy).not.toHaveBeenCalled();

    fireEvent.change(target, { target: { value: "copilot-user" } });
    expect(within(first).getByText("User deployment: this Skill will be installed for your account, not just one project.")).toBeTruthy();
    expect(save.disabled).toBe(false);
    fireEvent.click(within(first).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Deploy Skill" })).toBeNull();
    expect(deploy).not.toHaveBeenCalled();

    const second = await openDeployment("analyze-logs");
    const nextTarget = within(second).getByRole<HTMLSelectElement>("combobox", { name: "Skill deployment target" });
    expect(nextTarget.value).toBe("");
    expect(within(second).getByRole<HTMLButtonElement>("button", { name: "Deploy" }).disabled).toBe(true);
    fireEvent.change(nextTarget, { target: { value: "claude-project" } });
    expect(within(second).getByText("Project deployment: choose a project folder in the next step.")).toBeTruthy();
    expect(deploy).not.toHaveBeenCalled();
  });

  it("keeps the chosen destination and inline error on backend failure, then allows retry", async () => {
    const deploy = vi.fn()
      .mockRejectedValueOnce(new Error("Selected destination is read-only"))
      .mockResolvedValueOnce(deployed);
    showSkills(deploy);
    const dialog = await openDeployment("workspace-review");
    const target = within(dialog).getByRole<HTMLSelectElement>("combobox", { name: "Skill deployment target" });
    fireEvent.change(target, { target: { value: "agents-user" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Deploy" }));
    expect((await within(dialog).findByRole("alert")).textContent).toContain("Selected destination is read-only");
    expect(target.value).toBe("agents-user");
    expect(screen.getByRole("dialog", { name: "Deploy Skill" })).toBeTruthy();
    expect(deploy).toHaveBeenCalledWith(installations[0]!.installationId, "agents-user");
    fireEvent.click(within(dialog).getByRole("button", { name: "Deploy" }));
    await waitFor(() => expect(deploy).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Deploy Skill" })).toBeNull());
  });

  it("treats the native folder picker being canceled as no deployment", async () => {
    const deploy = vi.fn().mockResolvedValue(null);
    showSkills(deploy);
    const dialog = await openDeployment("workspace-review");
    const target = within(dialog).getByRole<HTMLSelectElement>("combobox", { name: "Skill deployment target" });
    fireEvent.change(target, { target: { value: "github-project" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Deploy" }));
    await waitFor(() => expect(deploy).toHaveBeenCalledOnce());
    await waitFor(() => expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Deploy" }).disabled).toBe(false));
    expect(screen.getByRole("dialog", { name: "Deploy Skill" })).toBeTruthy();
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(target.value).toBe("github-project");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Deploy Skill" })).toBeNull();
  });

  it("locks target and cancel while deployment is running", async () => {
    let finish!: (value: SkillDeployment | null) => void;
    const deploy = vi.fn().mockImplementation(() => new Promise<SkillDeployment | null>((resolve) => { finish = resolve; }));
    showSkills(deploy);
    const dialog = await openDeployment("workspace-review");
    const target = within(dialog).getByRole<HTMLSelectElement>("combobox", { name: "Skill deployment target" });
    fireEvent.change(target, { target: { value: "agents-user" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Deploy" }));
    await waitFor(() => expect(deploy).toHaveBeenCalledOnce());
    expect(target.disabled).toBe(true);
    expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled).toBe(true);
    expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Deploying…" }).disabled).toBe(true);
    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(screen.getByRole("dialog", { name: "Deploy Skill" })).toBeTruthy();
    finish(null);
    await waitFor(() => expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Deploy" }).disabled).toBe(false));
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  });

  it("shows native Chinese destination instructions without a preselected target", async () => {
    setLocale("zh-CN");
    const deploy = vi.fn();
    showSkills(deploy);
    fireEvent.click(await screen.findByRole("button", { name: "部署 workspace-review" }));
    const dialog = screen.getByRole("dialog", { name: "部署技能" });
    const target = within(dialog).getByRole<HTMLSelectElement>("combobox", { name: "技能部署目标" });
    expect(target.value).toBe("");
    expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "部署" }).disabled).toBe(true);
    fireEvent.change(target, { target: { value: "agents-user" } });
    expect(within(dialog).getByText("部署到当前用户：此技能可供当前账户使用，不局限于单个项目。")).toBeTruthy();
    expect(deploy).not.toHaveBeenCalled();
  });
});
