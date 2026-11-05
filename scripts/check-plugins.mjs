import { spawnSync } from "node:child_process";
import { mkdtempSync, realpathSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(import.meta.dirname, "..");
const bufCommand = "github.com/bufbuild/buf/cmd/buf@v1.73.0";
const generatedProtocols = [
  "sdk/providerplugin/go/protocol/v1",
  "sdk/sandboxplugin/go/protocol/v1",
  "sdk/tunnelplugin/go/protocol/v1",
];
const temporaryPrefix = "mender-plugin-check-";

const modules = {
  provider: {
    sdk: "sdk/providerplugin/go",
    example: "examples/provider-plugin-example",
    binary: "mender-provider-example",
  },
  sandbox: {
    sdk: "sdk/sandboxplugin/go",
    example: "examples/sandbox-plugin-example",
    binary: "mender-sandbox-example",
  },
  tunnel: {
    sdk: "sdk/tunnelplugin/go",
    example: "examples/tunnel-plugin-example",
    binary: "mender-tunnel-example",
  },
};

export const pluginTasks = {
  "provider-plugin:sdk:lint": ["provider", "sdk", ["lint"]],
  "provider-plugin:sdk:test": ["provider", "sdk", ["test"]],
  "provider-plugin:sdk:test:race": ["provider", "sdk", ["race"]],
  "provider-plugin:example:lint": ["provider", "example", ["lint"]],
  "provider-plugin:example:test": ["provider", "example", ["test"]],
  "provider-plugin:example:test:race": ["provider", "example", ["race"]],
  "provider-plugin:example:build": ["provider", "example", ["build"]],
  "sandbox-plugin:sdk:check": ["sandbox", "sdk", ["lint", "race"]],
  "sandbox-plugin:example:check": ["sandbox", "example", ["lint", "race", "build"]],
  "tunnel-plugin:sdk:check": ["tunnel", "sdk", ["lint", "race"]],
  "tunnel-plugin:example:check": ["tunnel", "example", ["lint", "race", "build"]],
};

export class PluginCheckFailure extends Error {
  constructor(command, args, result) {
    const detail = result.error?.message ??
      (result.signal ? `terminated by ${result.signal}` : `exit ${result.status}`);
    super(`${command} ${args.join(" ")} failed: ${detail}`);
    this.exitCode = Number.isInteger(result.status) && result.status > 0 ? result.status : 1;
  }
}

// argv, cwd and GOWORK are passed directly to executables. No task relies on
// shell environment assignments, directory changes or platform-specific rm.
export function runPluginCheck(task, {
  spawn = spawnSync,
  env = process.env,
  temporaryParent = tmpdir(),
} = {}) {
  const options = { env: { ...env, GOWORK: "off" }, stdio: "inherit" };
  function run(command, args, cwd = root) {
    const result = spawn(command, args, { ...options, cwd });
    if (result.error || result.status !== 0) {
      throw new PluginCheckFailure(command, args, result);
    }
  }
  function generateProtocols() {
    run("go", ["run", bufCommand, "generate", "--path", "contracts/providerplugin/v1/provider_plugin.proto"]);
    run("go", ["run", bufCommand, "generate", "--path", "contracts/sandboxplugin/v1/sandbox_plugin.proto", "--template", "buf.gen.sandbox.yaml"]);
    run("go", ["run", bufCommand, "generate", "--path", "contracts/tunnelplugin/v1/tunnel_plugin.proto", "--template", "buf.gen.tunnel.yaml"]);
  }

  if (task === "provider-plugin:proto:generate") {
    generateProtocols();
    return;
  }
  if (task === "provider-plugin:proto:check") {
    run("go", ["run", bufCommand, "lint"]);
    generateProtocols();
    run("git", ["diff", "--exit-code", "--", ...generatedProtocols]);
    return;
  }

  if (!Object.hasOwn(pluginTasks, task)) {
    throw new Error(`unknown plugin check: ${task ?? "(missing task)"}`);
  }
  const [kind, scope, actions] = pluginTasks[task];
  const module = modules[kind];
  const cwd = resolve(root, module[scope]);
  for (const action of actions) {
    if (action === "lint") {
      run("golangci-lint", ["run", "./..."], cwd);
    } else if (action === "test") {
      run("go", ["test", "./..."], cwd);
    } else if (action === "race") {
      run("go", ["test", "-race", "./..."], cwd);
    } else if (action === "build") {
      // Only a freshly created child of this resolved temporary parent may be
      // removed. Build failures still clean up, without touching example/bin.
      const parent = realpathSync(temporaryParent);
      const outputDirectory = mkdtempSync(join(parent, temporaryPrefix));
      try {
        const executable = module.binary + (process.platform === "win32" ? ".exe" : "");
        run("go", ["build", "-o", join(outputDirectory, executable), `./cmd/${module.binary}`], cwd);
      } finally {
        const resolvedOutput = realpathSync(outputDirectory);
        if (resolvedOutput !== outputDirectory || dirname(resolvedOutput) !== parent ||
            !basename(resolvedOutput).startsWith(temporaryPrefix)) {
          throw new Error(`refusing to remove unexpected build directory: ${resolvedOutput}`);
        }
        rmSync(resolvedOutput, { recursive: true, force: true, maxRetries: 3 });
      }
    }
  }
}

if (process.argv[1] && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))) {
  try {
    runPluginCheck(process.argv[2]);
  } catch (error) {
    console.error(error.message);
    process.exitCode = error instanceof PluginCheckFailure ? error.exitCode : 1;
  }
}
