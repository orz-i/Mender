import { resolve } from "node:path";
import { spawnSync } from "node:child_process";

const root = resolve(import.meta.dirname, "..");
const modules = ["backend", "apps/desktop", "sdk/sandboxplugin/go", "examples/sandbox-plugin-example", "sdk/tunnelplugin/go", "examples/tunnel-plugin-example"];

for (const directory of modules) {
  const cwd = resolve(root, directory);
  const env = { ...process.env, GOWORK: "off" };

  console.log(`go module isolation check: ${directory}`);

  runGo(cwd, env, ["mod", "tidy", "-diff"], false);
  runGo(cwd, env, ["list", "./..."], true);
}

function runGo(cwd, env, args, quiet) {
  const result = spawnSync("go", args, {
    cwd,
    env,
    stdio: quiet ? ["ignore", "ignore", "inherit"] : "inherit",
  });

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}
