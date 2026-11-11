import { spawn } from "node:child_process";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const exePath = process.platform === "win32"
  ? resolve(root, "apps/desktop/bin/mender-desktop.exe")
  : resolve(root, "apps/desktop/bin/mender-desktop");

const child = spawn(exePath, [], {
  cwd: resolve(root, "apps/desktop"),
  stdio: "inherit",
});

child.on("exit", (code) => {
  process.exit(code ?? 0);
});
