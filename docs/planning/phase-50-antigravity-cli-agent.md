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

No production Agent/credential state is changed by tests. Full gate and final commits are recorded at close.
