import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { spawnSync } from "node:child_process";

const root = resolve(import.meta.dirname, "..");
const version = readFileSync(resolve(root, ".govulncheck-version"), "utf8").trim();

if (!/^v\d+\.\d+\.\d+$/.test(version)) {
  console.error(`invalid govulncheck version: ${version}`);
  process.exit(1);
}

for (const directory of [
  "backend",
  "apps/desktop",
  "sdk/providerplugin/go",
  "examples/provider-plugin-example",
]) {
  console.log(`govulncheck ${version}: ${directory}`);
  const result = spawnSync(
    "go",
    [
      "run",
      `golang.org/x/vuln/cmd/govulncheck@${version}`,
      "./...",
    ],
    {
      cwd: resolve(root, directory),
      env: {
        ...process.env,
        ...(directory.startsWith("sdk/") || directory.startsWith("examples/")
          ? { GOWORK: "off" }
          : {}),
      },
      stdio: "inherit",
    },
  );
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}
