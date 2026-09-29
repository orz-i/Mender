import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { HashRouter } from "react-router";

import { MenderApp, t } from "@mender/app";
import { Clipboard, Dialogs } from "@wailsio/runtime";

import "@mender/ui/globals.css";

import {
  createDesktopMenderClient,
  installDesktopProviderPlugin,
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

createRoot(rootElement).render(
  <StrictMode>
    <HashRouter>
      <MenderApp
        client={client}
        copyText={Clipboard.SetText}
        installProviderPlugin={chooseAndInstallProviderPlugin}
      />
    </HashRouter>
  </StrictMode>,
);
