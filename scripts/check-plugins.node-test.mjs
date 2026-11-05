import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readdirSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import test from "node:test";

import { PluginCheckFailure, pluginTasks, runPluginCheck } from "./check-plugins.mjs";

const root = resolve(import.meta.dirname, "..");

test("module checks retain isolated env, exact argv and every quality gate", () => {
  const expected = {
    "provider-plugin:sdk:lint": ["sdk/providerplugin/go", [["golangci-lint", "run", "./..."]]],
    "provider-plugin:sdk:test": ["sdk/providerplugin/go", [["go", "test", "./..."]]],
    "provider-plugin:sdk:test:race": ["sdk/providerplugin/go", [["go", "test", "-race", "./..."]]],
    "provider-plugin:example:lint": ["examples/provider-plugin-example", [["golangci-lint", "run", "./..."]]],
    "provider-plugin:example:test": ["examples/provider-plugin-example", [["go", "test", "./..."]]],
    "provider-plugin:example:test:race": ["examples/provider-plugin-example", [["go", "test", "-race", "./..."]]],
    "sandbox-plugin:sdk:check": ["sdk/sandboxplugin/go", [["golangci-lint", "run", "./..."], ["go", "test", "-race", "./..."]]],
    "sandbox-plugin:example:check": ["examples/sandbox-plugin-example", [["golangci-lint", "run", "./..."], ["go", "test", "-race", "./..."], ["go", "build"]]],
    "tunnel-plugin:sdk:check": ["sdk/tunnelplugin/go", [["golangci-lint", "run", "./..."], ["go", "test", "-race", "./..."]]],
    "tunnel-plugin:example:check": ["examples/tunnel-plugin-example", [["golangci-lint", "run", "./..."], ["go", "test", "-race", "./..."], ["go", "build"]]],
    "provider-plugin:example:build": ["examples/provider-plugin-example", [["go", "build"]]],
  };
  assert.deepEqual(Object.keys(pluginTasks).sort(), Object.keys(expected).sort());
  for (const [task, [directory, commands]] of Object.entries(expected)) {
    const calls = [];
    const env = { PATH: "fixture-path", GOWORK: "unexpected-workspace" };
    runPluginCheck(task, {
      env,
      spawn(command, args, options) {
        calls.push([command, ...args]);
        assert.equal(options.cwd, resolve(root, directory));
        assert.deepEqual(options.env, { PATH: "fixture-path", GOWORK: "off" });
        assert.equal(options.shell, undefined);
        return { status: 0 };
      },
    });
    assert.equal(env.GOWORK, "unexpected-workspace", "must not mutate caller environment");
    assert.equal(calls.length, commands.length, task);
    for (let index = 0; index < commands.length; index++) {
      const wanted = commands[index];
      assert.deepEqual(calls[index].slice(0, wanted.length), wanted, task);
      if (wanted[1] !== "build") assert.deepEqual(calls[index], wanted, task);
    }
  }
});

test("protocol check lints, regenerates all contracts and verifies committed output", () => {
  const calls = [];
  runPluginCheck("provider-plugin:proto:check", {
    spawn(command, args, options) {
      calls.push([command, ...args]);
      assert.equal(options.cwd, root);
      assert.equal(options.env.GOWORK, "off");
      return { status: 0 };
    },
  });
  const buf = "github.com/bufbuild/buf/cmd/buf@v1.73.0";
  assert.deepEqual(calls, [
    ["go", "run", buf, "lint"],
    ["go", "run", buf, "generate", "--path", "contracts/providerplugin/v1/provider_plugin.proto"],
    ["go", "run", buf, "generate", "--path", "contracts/sandboxplugin/v1/sandbox_plugin.proto", "--template", "buf.gen.sandbox.yaml"],
    ["go", "run", buf, "generate", "--path", "contracts/tunnelplugin/v1/tunnel_plugin.proto", "--template", "buf.gen.tunnel.yaml"],
    ["git", "diff", "--exit-code", "--", "sdk/providerplugin/go/protocol/v1", "sdk/sandboxplugin/go/protocol/v1", "sdk/tunnelplugin/go/protocol/v1"],
  ]);
  const generation = [];
  runPluginCheck("provider-plugin:proto:generate", {
    spawn(command, args) {
      generation.push([command, ...args]);
      return { status: 0 };
    },
  });
  assert.deepEqual(generation, calls.slice(1, 4));
});

test("a failed gate stops subsequent commands and preserves its exit code", () => {
  let calls = 0;
  assert.throws(() => runPluginCheck("sandbox-plugin:example:check", {
    spawn() {
      calls++;
      return { status: 7 };
    },
  }), (error) => error instanceof PluginCheckFailure && error.exitCode === 7);
  assert.equal(calls, 1);
  assert.throws(() => runPluginCheck("provider-plugin:sdk:test", {
    spawn() { return { status: null, error: new Error("ENOENT fixture") }; },
  }), /ENOENT fixture/);
  let protocolCalls = 0;
  assert.throws(() => runPluginCheck("provider-plugin:proto:check", {
    spawn() {
      protocolCalls++;
      return { status: protocolCalls === 2 ? 9 : 0 };
    },
  }), (error) => error.exitCode === 9);
  assert.equal(protocolCalls, 2, "failed generation must stop the protocol gate");
});

test("example builds clean only their temporary output on success and failure", () => {
  const temporaryParent = mkdtempSync(join(realpathSync(tmpdir()), "mender-plugin-test-"));
  const sentinel = join(temporaryParent, "keep-me.txt");
  writeFileSync(sentinel, "preserve unrelated files");
  try {
    for (const status of [0, 3]) {
      let outputDirectory;
      const build = () => runPluginCheck("provider-plugin:example:build", {
        temporaryParent,
        spawn(command, args) {
          assert.equal(command, "go");
          assert.equal(args[0], "build");
          const output = args[args.indexOf("-o") + 1];
          outputDirectory = dirname(output);
          assert.equal(dirname(outputDirectory), temporaryParent);
          assert.match(basename(outputDirectory), /^mender-plugin-check-/);
          assert.equal(args.at(-1), "./cmd/mender-provider-example");
          writeFileSync(output, "partial build output");
          return { status };
        },
      });
      if (status === 0) build();
      else assert.throws(build, (error) => error.exitCode === status);
      assert.deepEqual(readdirSync(temporaryParent), ["keep-me.txt"]);
      assert.ok(outputDirectory);
    }
  } finally {
    const parent = realpathSync(tmpdir());
    const resolved = realpathSync(temporaryParent);
    assert.equal(dirname(resolved), parent);
    assert.match(basename(resolved), /^mender-plugin-test-/);
    rmSync(resolved, { recursive: true, force: true });
  }
});

test("unknown tasks cannot launch an executable", () => {
  for (const task of [undefined, "toString", "provider-plugin:example:arbitrary"]) {
    assert.throws(() => runPluginCheck(task, {
      spawn() { assert.fail("unexpected executable launch"); },
    }), /unknown plugin check/);
  }
});

test("the CLI rejects an invalid task with a failing process status", () => {
  const result = spawnSync(process.execPath, [resolve(root, "scripts/check-plugins.mjs"), "unknown-task"], {
    cwd: root,
    encoding: "utf8",
  });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /unknown plugin check: unknown-task/);
});
