import { writeFileSync } from "node:fs";

import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";
import { defineConfig } from "vite";

export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [
    react(),
    wails("./bindings"),
    {
      name: "mender-keep-dist-placeholder",
      closeBundle() {
        writeFileSync(new URL("./dist/.gitkeep", import.meta.url), "");
      },
    },
  ],
});
