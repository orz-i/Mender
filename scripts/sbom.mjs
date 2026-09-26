import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { spawnSync } from "node:child_process";

const root = resolve(import.meta.dirname, "..");
const version = readFileSync(resolve(root, ".syft-version"), "utf8").trim();
const target = resolve(root, process.argv[2] ?? "dist/release/go");
const output = resolve(
  root,
  process.argv[3] ?? "dist/release/mender-go.spdx.json",
);

if (!/^v\d+\.\d+\.\d+$/.test(version)) {
  console.error(`invalid Syft version: ${version}`);
  process.exit(2);
}

mkdirSync(dirname(output), { recursive: true });

const metadataPath = resolve(target, "build-metadata.json");
let sourceVersion = "unknown";
if (existsSync(metadataPath)) {
  const metadata = JSON.parse(readFileSync(metadataPath, "utf8"));
  if (typeof metadata.version === "string" && metadata.version !== "") {
    sourceVersion = metadata.version;
  }
}

const result = spawnSync(
  "go",
  [
    "run",
    `github.com/anchore/syft/cmd/syft@${version}`,
    target,
    "--source-name",
    "Mender",
    "--source-version",
    sourceVersion,
    "-o",
    `spdx-json=${output}`,
  ],
  {
    cwd: root,
    stdio: "inherit",
  },
);
if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

console.log(`SBOM written to ${output} with Syft ${version}`);
