# Phase 50 — Antigravity CLI native Agent backend

Date: 2026-10-18. Authority: [ADR 0050](../architecture/adr/0050-antigravity-headless-agent-backend.md).

## Intent contract

An owner with an already-authenticated local `agy` can add an Antigravity Agent from the built-in plugin marketplace and delegate a real Agent Task to its approved local workspace. Keep all existing Mender Agent admission, lifecycle and Profile-supply boundaries. Mender is not the Agent Loop. Antigravity is headless JSON, **not ACP**: native permission prompts cannot be satisfied in the Mender ACP approval inbox. No implicit global approval or new compatibility shim. The owner explicitly approved the `agy --dangerously-skip-permissions` capability: only a per-Agent/per-workspace 24h trusted grant enables it, with explicit user acknowledgement and no ACP approval receipt.

## Work clusters / checks

| Cluster | Output | Passing check |
|---|---|---|
| Backend identity and persistence | v13 forward migration, literal builtin plugin, trusted `agy` launch | Go schema/domain/launcher tests; FK check; preexisting identities unchanged |
| Headless transport | Native Task Executor and model inspector routed only for `antigravity` | Structured stream, failures, bounds, cancellation, ToolSupply denial tests |
| User-facing surface | OpenAPI SDK, Wails, React, i18n and CLI | Typecheck, marketplace and creation, i18n tests |
| Integration | Native no-tool smoke, full `pnpm check`, evidence | Measured result; no production DB changes; clean tracked git status |
| YOLO opt-in | Reuse trusted workspace policy and literal flag; default never bypasses | Fixture true/false/expiry/other workspace; real CLI init mode observed |

## Discoveries

- Installed binary: `/Users/mini/.local/bin/agy` v1.4.2, authenticated interactively by the owner (not probed or copied by Mender).
- `agy models` in v1.4.2 outputs `<slug>\t<display name>`; `models --output-format json` is **not** supported by this binary despite newer documentation describing such a flag.
- The CLI exposes `--input-format stream-json`/`--output-format stream-json`. No `agy acp` subcommand.
- Third-party `agy-acp` adapters are not used; they differ from the official headless interface and would add an installation and protocol review surface.

## Evidence and outstanding gates

Implementation evidence so far (before full quality gate):

- `agy models` locally returns six models; Mender's native inspector parses exact slugs and display names without prompts.
- Real CLI no-tool headless JSON returned the exact marker with `SUCCESS`, exit 0 and five parsed events, using the intentionally narrowed Mender Host environment and ephemeral workspace.
- Opt-in Mender native launcher and Antigravity runner test completed real no-tool `InspectModels` and `ExecuteConfigured`, exit 0, ~16 seconds, no workspace writes.
- Real CLI started with `--dangerously-skip-permissions` in a temporary workspace (no native tool invoked) advertised `init.permission_mode=always-proceed` and completed an exact no-tool marker successfully. This **does not** prove unrestricted tools are confined to a workspace; the documented flag explicitly bypasses native permission prompts.
- Backend fixture coverage: secure argv selection, invalid flags, protected credentials, output frames/terminal result, cancel, native tool-supply denial, trust off/trust on/trust revoked/trust expired/other workspace; synthetic database v12→v13 preserves historic task/grant/policy/agent identities with FK and integrity validation.
- Frontend Vitest: 46 files, 279 passed; i18n 1349/1349; TS typecheck passed.

## Completion evidence (2026-10-18)

- **Five staged commits before close:** `d3b7c69` (ADR and phase contract), `d54d9f1` (owner YOLO clarification documented), `7ddc401` (trusted launcher, backend router, schema v13, transport/permission fixtures), `df2f211` (SDK/OpenAPI/Wails/React/i18n), `10543d1` (desktop catalogue contract).
- **Full canonical quality gate:** `pnpm check` **exit 0**, receipt `/tmp/mender-phase50-final-check.exit`; all Go unit tests and Race Detector, plugin SDKs/examples, buf/code generation, `go:lint` **0 issues**, Wails bindings and architecture, TypeScript/ESLint, i18n **1350/1350**, Vitest **46 files / 279 passed**, Web/Desktop production builds. Existing frontend chunk-size warnings are nonfatal.
- **Native macOS desktop compilation:** `cd apps/desktop && go build -o /tmp/mender-phase50-desktop-qa .` exit 0; private QA executable was removed after verification. The system linker emitted nonblocking warnings about differing macOS object deployment targets.
- **Live CLI identity/transport:** the authenticated owner CLI v1.4.2 successfully completed a real no-tool headless prompt under the same fixed, minimal Host process environment, exiting 0 with a single SUCCESS result and exact token. An **opt-in Go integration test** using Mender's real `LocalFactory.AntigravityCommand` and `antigravity.Runner` completed no-prompt model inspection plus one exact no-tool task against the real installed `agy`, without entering the production task inventory.
- **Live YOLO flag:** `agy --dangerously-skip-permissions` in a fresh ephemeral workspace advertised `init.permission_mode=always-proceed`, exited 0 and returned the exact no-tool marker. The test did not call write/shell tools or assert filesystem confinement; host authority remains documented.
- **Deterministic authorization:** fixtures verify default mode has no bypass, only owner-granted workspace `trusted` adds YOLO to the exact Agent/cwd at process launch; other workspaces, revoked/expired grants, forged model/effort flags and model-inspection calls do not receive bypass. AGY has no ACP request_permission bridge, and governed ToolSupply is refused before subprocess execution. Cancellation, malformed/duplicate/incomplete/failed events and output limits fail closed.
- **Persistence and history:** a synthetic populated Agent v12 database migrated to v13 while preserving a historic Grok Agent model/effort, completed Task result, Profile grant and trusted policy; SQLite `quick_check=ok`, FK check zero violations, new Antigravity Agent accepted by v13 CHECK and unknown backend rejected. Existing v7/v8/v9/v10/v11/v12 migration test fixtures remain passing.
- **Owner rollout boundary:** this source and QA binary are tested. The running owner's Wails desktop has **not** been rebuilt/reconnected to this HEAD, and no new production Antigravity Agent or Profile grant has been automatically created. Owner must rebuild/reconnect, add an Antigravity Agent from marketplace, choose isolation/workspace and optionally grant scoped 24h YOLO. No account credentials, old agent records, task results or production permission grants were rewritten during implementation.

No production Agent/credential state is changed by these tests. The four pre-existing untracked skill/experiment directories remain untouched and the branch was not pushed.
