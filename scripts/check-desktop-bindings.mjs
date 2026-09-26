import { resolve } from "node:path";
import { spawnSync } from "node:child_process";

const root = resolve(import.meta.dirname, "..");
const desktop = resolve(root, "apps/desktop");
const wailsVersion = "v3.0.0-beta.26";

function run(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    stdio: "inherit",
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

run(
  "go",
  [
    "run",
    `github.com/wailsapp/wails/v3/cmd/wails3@${wailsVersion}`,
    "generate",
    "bindings",
    "-ts",
    "-i",
  ],
  desktop,
);

run(
  "git",
  [
    "diff",
    "--exit-code",
    "--",
    "apps/desktop/frontend/bindings",
  ],
  root,
);

console.log("desktop bindings check: ok");
