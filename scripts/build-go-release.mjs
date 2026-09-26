import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { spawnSync } from "node:child_process";

const root = resolve(import.meta.dirname, "..");
const backend = resolve(root, "backend");
const outputRoot = resolve(
  root,
  process.env.MENDER_RELEASE_DIR ?? "dist/release/go",
);

const version = process.env.MENDER_VERSION ?? process.argv[2];
if (
  version === undefined ||
  !/^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(version)
) {
  console.error("MENDER_VERSION must be a SemVer tag such as v1.2.3");
  process.exit(2);
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? root,
    env: options.env ?? process.env,
    encoding: options.capture ? "utf8" : undefined,
    stdio: options.capture ? ["ignore", "pipe", "pipe"] : "inherit",
  });
  if (result.status !== 0) {
    if (options.capture && result.stderr) {
      process.stderr.write(result.stderr);
    }
    process.exit(result.status ?? 1);
  }
  return options.capture ? result.stdout.trim() : "";
}

if (process.env.MENDER_ALLOW_DIRTY !== "1") {
  const dirty = run("git", ["status", "--porcelain"], { capture: true });
  if (dirty !== "") {
    console.error(
      "release build requires a clean working tree; set MENDER_ALLOW_DIRTY=1 only for local verification",
    );
    process.exit(2);
  }
}

const commit =
  process.env.MENDER_COMMIT ??
  run("git", ["rev-parse", "HEAD"], { capture: true });
const buildDate =
  process.env.MENDER_BUILD_DATE ??
  run("git", ["show", "-s", "--format=%cI", commit], { capture: true });

const defaultTargets = [
  "linux/amd64",
  "linux/arm64",
  "darwin/amd64",
  "darwin/arm64",
  "windows/amd64",
  "windows/arm64",
];
const targets = (process.env.MENDER_TARGETS ?? defaultTargets.join(","))
  .split(",")
  .map((value) => value.trim())
  .filter(Boolean);

rmSync(outputRoot, { recursive: true, force: true });
mkdirSync(outputRoot, { recursive: true });

const ldflags = [
  "-s",
  "-w",
  "-X",
  `github.com/orz-i/Mender/backend/internal/buildinfo.Version=${version}`,
  "-X",
  `github.com/orz-i/Mender/backend/internal/buildinfo.Commit=${commit}`,
  "-X",
  `github.com/orz-i/Mender/backend/internal/buildinfo.BuildDate=${buildDate}`,
].join(" ");

for (const target of targets) {
  const [goos, goarch, extra] = target.split("/");
  if (
    extra !== undefined ||
    goos === undefined ||
    goarch === undefined ||
    !["linux", "darwin", "windows"].includes(goos) ||
    !["amd64", "arm64"].includes(goarch)
  ) {
    console.error(`unsupported release target: ${target}`);
    process.exit(2);
  }

  const extension = goos === "windows" ? ".exe" : "";

  for (const command of ["mender", "menderd"]) {
    const output = resolve(
      outputRoot,
      `${command}-${version}-${goos}-${goarch}${extension}`,
    );
    console.log(`building ${command} ${goos}/${goarch}`);
    run(
      "go",
      [
        "build",
        "-trimpath",
        "-buildvcs=false",
        "-ldflags",
        ldflags,
        "-o",
        output,
        `./cmd/${command}`,
      ],
      {
        cwd: backend,
        env: {
          ...process.env,
          CGO_ENABLED: "0",
          GOOS: goos,
          GOARCH: goarch,
        },
      },
    );
  }
}

writeFileSync(
  resolve(outputRoot, "build-metadata.json"),
  JSON.stringify(
    {
      version,
      commit,
      buildDate,
      targets,
      goVersion: run("go", ["version"], { capture: true }),
    },
    null,
    2,
  ) + "\n",
);

console.log(`release binaries written to ${outputRoot}`);
