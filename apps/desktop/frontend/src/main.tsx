import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { HashRouter } from "react-router";

import { MenderApp, t } from "@mender/app";
import type { ExportProfilePluginInput, SkillDeployment } from "@mender/sdk";
import { Browser, Clipboard, Dialogs } from "@wailsio/runtime";

import "@mender/ui/globals.css";

import {
  createDesktopMenderClient,
  deployDesktopProfileSkills,
  exportDesktopProfilePlugin,
  getDesktopProfileCLI,
  installDesktopPlugin,
  installDesktopSkill,
} from "./desktop-client";
import { createDesktopLocalOwnerCapabilities } from "./desktop-local-owner";

const rootElement = document.getElementById("root");

if (rootElement === null) {
  throw new Error("Mender desktop root element was not found");
}

const client = createDesktopMenderClient();
const localOwner = createDesktopLocalOwnerCapabilities();

async function chooseLocalSandboxDirectory(): Promise<string | null> {
  const directory = await Dialogs.OpenFile({
    AllowsMultipleSelection: false,
    CanChooseDirectories: true,
    CanChooseFiles: false,
    CanCreateDirectories: false,
    Title: t("Choose local directory"),
    Message: t("Choose the workspace directory for this local Sandbox."),
    ButtonText: t("Select directory"),
  });
  return directory || null;
}

async function chooseAndInstallPlugin() {
  const sourcePath = await Dialogs.OpenFile({
    AllowsMultipleSelection: false,
    CanChooseDirectories: true,
    CanChooseFiles: false,
    CanCreateDirectories: false,
    Title: t("Install plugin"),
    Message: t("Choose a plugin package directory."),
    ButtonText: t("Install"),
  });
  if (!sourcePath) {
    return null;
  }
  return installDesktopPlugin(sourcePath);
}

async function chooseAndInstallSkill() {
  const sourcePath = await Dialogs.OpenFile({
    AllowsMultipleSelection: false,
    CanChooseDirectories: true,
    CanChooseFiles: false,
    CanCreateDirectories: false,
    Title: t("Install Skill"),
    Message: t("Choose an Agent Skill directory."),
    ButtonText: t("Install"),
  });
  if (!sourcePath) {
    return null;
  }
  return installDesktopSkill(sourcePath);
}

async function chooseAndDeployProfileSkills(
  slug: string,
  target: SkillDeployment["target"],
): Promise<SkillDeployment[] | null> {
  let projectRoot = "";
  if (target.endsWith("-project")) {
    projectRoot = await Dialogs.OpenFile({
      AllowsMultipleSelection: false,
      CanChooseDirectories: true,
      CanChooseFiles: false,
      CanCreateDirectories: false,
      Title: t("Deploy Profile skills"),
      Message: t("Choose the project root for the selected Profile Skills."),
      ButtonText: t("Deploy"),
    });
    if (!projectRoot) return null;
  }
  return deployDesktopProfileSkills(slug, target, projectRoot);
}

async function chooseAndExportProfilePlugin(slug: string, input: ExportProfilePluginInput) {
  const outputParent = await Dialogs.OpenFile({
    AllowsMultipleSelection: false,
    CanChooseDirectories: true,
    CanChooseFiles: false,
    CanCreateDirectories: false,
    Title: t("Export Profile Agent Plugin"),
    Message: t("Choose the parent directory for a new portable Agent Plugin package."),
    ButtonText: t("Choose folder"),
  });
  if (!outputParent) return null;
  return exportDesktopProfilePlugin(slug, input, outputParent);
}

createRoot(rootElement).render(
  <StrictMode>
    <HashRouter>
      <MenderApp
        client={client}
        localOwner={localOwner}
        openExternalURL={Browser.OpenURL}
        getLocalProfileCLI={getDesktopProfileCLI}
        copyText={Clipboard.SetText}
        chooseLocalDirectory={chooseLocalSandboxDirectory}
        installPlugin={chooseAndInstallPlugin}
        installSkill={chooseAndInstallSkill}
        deployProfileSkills={chooseAndDeployProfileSkills}
        exportProfilePlugin={chooseAndExportProfilePlugin}
      />
    </HashRouter>
  </StrictMode>,
);
