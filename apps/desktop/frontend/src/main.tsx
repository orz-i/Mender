import React from "react";
import ReactDOM from "react-dom/client";

import App from "./App";

const root = document.getElementById("root");
if (root === null) {
  throw new Error("Mender desktop root element was not found");
}

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
