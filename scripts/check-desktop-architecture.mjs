import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { spawnSync } from "node:child_process";

const root = resolve(import.meta.dirname, "..");

function fail(message) {
  console.error(`desktop architecture check: ${message}`);
  process.exit(1);
}

function run(command, args, cwd = root) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
  if (result.status !== 0) {
    process.stderr.write(result.stderr);
    fail(`${command} ${args.join(" ")} failed`);
  }
  return result.stdout;
}

function sourceFiles(directory) {
  const result = [];
  for (const entry of readdirSync(directory)) {
    if (entry === "node_modules" || entry === "dist" || entry === "bindings") {
      continue;
    }
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) {
      result.push(...sourceFiles(path));
      continue;
    }
    if (/\.(?:[cm]?[jt]s|tsx)$/.test(entry)) {
      result.push(path);
    }
  }
  return result;
}

const backendGoMod = readFileSync(join(root, "backend/go.mod"), "utf8");
if (backendGoMod.includes("github.com/wailsapp/wails")) {
  fail("backend/go.mod must not depend on Wails");
}

const commandDeps = run("go", [
  "list",
  "-deps",
  "./backend/cmd/mender",
  "./backend/cmd/menderd",
]);
if (commandDeps.split(/\r?\n/).some((line) => line.toLowerCase().includes("wails"))) {
  fail("CLI or daemon dependency graph contains Wails");
}

for (const directory of ["packages/app", "packages/sdk"]) {
  for (const file of sourceFiles(join(root, directory))) {
    const content = readFileSync(file, "utf8");
    if (content.includes("@wailsio/") || content.includes("/bindings/")) {
      fail(`${relative(root, file)} imports desktop/Wails bindings`);
    }
  }
}

for (const directory of ["apps", "packages"]) {
  for (const file of sourceFiles(join(root, directory))) {
    const relativePath = relative(root, file).replaceAll("\\", "/");
    if (relativePath.startsWith("packages/sdk/")) {
      continue;
    }
    const content = readFileSync(file, "utf8");
    if (/\bfetch\s*\(/.test(content)) {
      fail(`${relativePath} calls fetch outside packages/sdk`);
    }
  }
}

const webMain = readFileSync(join(root, "apps/web/src/main.tsx"), "utf8");
if (!webMain.includes("BrowserRouter")) {
  fail("browser host must use BrowserRouter");
}

const desktopMain = readFileSync(
  join(root, "apps/desktop/frontend/src/main.tsx"),
  "utf8",
);
if (!desktopMain.includes("HashRouter")) {
  fail("desktop host must use HashRouter");
}
if (!desktopMain.includes('from "@mender/app"')) {
  fail("desktop host must mount the shared @mender/app product UI");
}

const darwinTask = readFileSync(
  join(root, "apps/desktop/build/darwin/Taskfile.yml"),
  "utf8",
);
for (const expected of [
  "-mmacosx-version-min=13.0",
  'MACOSX_DEPLOYMENT_TARGET: "13.0"',
]) {
  if (!darwinTask.includes(expected)) {
    fail(`macOS build assets must preserve Go 1.27 deployment target: ${expected}`);
  }
}
for (const file of ["Info.plist", "Info.dev.plist"]) {
  const plist = readFileSync(
    join(root, "apps/desktop/build/darwin", file),
    "utf8",
  );
  if (!plist.includes("<string>13.0.0</string>")) {
    fail(`${file} must declare macOS 13.0 minimum system version`);
  }
}

console.log("desktop architecture check: ok");
