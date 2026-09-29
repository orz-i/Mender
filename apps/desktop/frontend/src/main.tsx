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
  installDesktopProviderPlugin,
  installDesktopSkill,
} from "./desktop-client";

const rootElement = document.getElementById("root");

if (rootElement === null) {
  throw new Error("Mender desktop root element was not found");
}

const client = createDesktopMenderClient();

async function chooseAndInstallProviderPlugin() {
  const sourcePath = await Dialogs.OpenFile({
    AllowsMultipleSelection: false,
    CanChooseDirectories: true,
    CanChooseFiles: false,
    CanCreateDirectories: false,
    Title: t("Install Provider plugin"),
    Message: t("Choose a Provider plugin package directory."),
    ButtonText: t("Install"),
  });
  if (!sourcePath) {
    return null;
  }
  return installDesktopProviderPlugin(sourcePath);
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
        installProviderPlugin={chooseAndInstallProviderPlugin}
        installSkill={chooseAndInstallSkill}
        deploySkill={chooseAndDeploySkill}
      />
    </HashRouter>
  </StrictMode>,
);
