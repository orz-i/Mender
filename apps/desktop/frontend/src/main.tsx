import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { HashRouter } from "react-router";

import { MenderApp } from "@mender/app";

import "@mender/ui/globals.css";

import { createDesktopMenderClient } from "./desktop-client";

const rootElement = document.getElementById("root");

if (rootElement === null) {
  throw new Error("Mender desktop root element was not found");
}

const client = createDesktopMenderClient();

createRoot(rootElement).render(
  <StrictMode>
    <HashRouter>
      <MenderApp client={client} />
    </HashRouter>
  </StrictMode>,
);
