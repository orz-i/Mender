import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { HashRouter } from "react-router";

import { MenderApp, t } from "@mender/app";
import type { SkillDeployment } from "@mender/sdk";
import { Clipboard, Dialogs } from "@wailsio/runtime";

import "@mender/ui/globals.css";

import {
  createDesktopMenderClient,
  deployDesktopSkill,
  installDesktopPlugin,
  installDesktopSkill,
} from "./desktop-client";

const rootElement = document.getElementById("root");

if (rootElement === null) {
  throw new Error("Mender desktop root element was not found");
}

const client = createDesktopMenderClient();

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

async function chooseAndDeploySkill(
  installationId: string,
  target: SkillDeployment["target"],
) {
  let projectRoot = "";
  if (target.endsWith("-project")) {
    projectRoot = await Dialogs.OpenFile({
      AllowsMultipleSelection: false,
      CanChooseDirectories: true,
      CanChooseFiles: false,
      CanCreateDirectories: false,
      Title: t("Deploy Skill"),
      Message: t("Choose the project root for this Skill deployment."),
      ButtonText: t("Deploy"),
    });
    if (!projectRoot) {
      return null;
    }
  }
  return deployDesktopSkill(installationId, target, projectRoot);
}

createRoot(rootElement).render(
  <StrictMode>
    <HashRouter>
      <MenderApp
        client={client}
        copyText={Clipboard.SetText}
        chooseLocalDirectory={chooseLocalSandboxDirectory}
        installPlugin={chooseAndInstallPlugin}
        installSkill={chooseAndInstallSkill}
        deploySkill={chooseAndDeploySkill}
      />
    </HashRouter>
  </StrictMode>,
);
