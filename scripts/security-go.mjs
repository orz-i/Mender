import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { spawnSync } from "node:child_process";

const root = resolve(import.meta.dirname, "..");
const version = readFileSync(resolve(root, ".govulncheck-version"), "utf8").trim();
const primaryDatabase = process.env.GOVULNDB ?? "https://vuln.go.dev";
const fallbackDatabase =
  process.env.GOVULNDB_FALLBACK ??
  "https://storage.googleapis.com/go-vulndb";

if (!/^v\d+\.\d+\.\d+$/.test(version)) {
  console.error(`invalid govulncheck version: ${version}`);
  process.exit(1);
}

let database = primaryDatabase;
if (!(await databaseReachable(primaryDatabase))) {
  console.warn(
    `Go vulnerability database ${primaryDatabase} is unavailable; using ${fallbackDatabase}`,
  );
  database = fallbackDatabase;
}

for (const directory of [
  "backend",
  "apps/desktop",
  "sdk/providerplugin/go",
  "examples/provider-plugin-example",
  "sdk/sandboxplugin/go",
  "examples/sandbox-plugin-example",
  "sdk/tunnelplugin/go",
  "examples/tunnel-plugin-example",
]) {
  console.log(`govulncheck ${version}: ${directory}`);
  const cwd = resolve(root, directory);
  const env = {
    ...process.env,
    ...(directory.startsWith("sdk/") || directory.startsWith("examples/")
      ? { GOWORK: "off" }
      : {}),
  };

  const primary = runGovulncheck(cwd, env, database);
  printResult(primary);
  if (primary.status === 0) {
    continue;
  }

  if (
    database !== fallbackDatabase &&
    isDatabaseAvailabilityFailure(primary)
  ) {
    console.warn(
      `Go vulnerability database ${database} became unavailable; retrying with ${fallbackDatabase}`,
    );
    database = fallbackDatabase;
    const fallback = runGovulncheck(cwd, env, database);
    printResult(fallback);
    if (fallback.status === 0) {
      continue;
    }
    process.exit(fallback.status ?? 1);
  }

  process.exit(primary.status ?? 1);
}

async function databaseReachable(database) {
  try {
    const response = await fetch(`${database}/index/db.json.gz`, {
      method: "GET",
      signal: AbortSignal.timeout(5_000),
    });
    await response.body?.cancel();
    return response.ok;
  } catch {
    return false;
  }
}

function runGovulncheck(cwd, env, database) {
  return spawnSync(
    "go",
    [
      "run",
      `golang.org/x/vuln/cmd/govulncheck@${version}`,
      `-db=${database}`,
      "./...",
    ],
    {
      cwd,
      env,
      encoding: "utf8",
      maxBuffer: 16 * 1024 * 1024,
    },
  );
}

function printResult(result) {
  if (result.stdout) {
    process.stdout.write(result.stdout);
  }
  if (result.stderr) {
    process.stderr.write(result.stderr);
  }
  if (result.error) {
    console.error(result.error);
  }
}

function isDatabaseAvailabilityFailure(result) {
  const output = `${result.stdout ?? ""}\n${result.stderr ?? ""}`;
  return /fetching vulnerabilities|dial tcp|connection (?:timed out|reset|refused)|tls handshake timeout|no such host|temporary failure|i\/o timeout|unexpected eof/i.test(
    output,
  );
}
