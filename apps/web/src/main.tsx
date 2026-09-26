import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";

import { MenderApp } from "@mender/app";

import "@mender/ui/globals.css";

const rootElement = document.getElementById("root");

if (rootElement === null) {
  throw new Error("Mender root element was not found");
}

createRoot(rootElement).render(
  <StrictMode>
    <BrowserRouter>
      <MenderApp />
    </BrowserRouter>
  </StrictMode>,
);
