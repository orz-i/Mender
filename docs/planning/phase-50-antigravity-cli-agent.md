# Phase 50 — Antigravity CLI native Agent backend

Date: 2026-10-18. Authority: [ADR 0050](../architecture/adr/0050-antigravity-headless-agent-backend.md).

## Intent contract

An owner with an already-authenticated local `agy` can add an Antigravity Agent from the built-in plugin marketplace and delegate a real Agent Task to its approved local workspace. Keep all existing Mender Agent admission, lifecycle and Profile-supply boundaries. Mender is not the Agent Loop. Antigravity is headless JSON, **not ACP**: native permission prompts cannot be satisfied in the Mender ACP approval inbox. No implicit global approval or new compatibility shim.

## Work clusters / checks

| Cluster | Output | Passing check |
|---|---|---|
| Backend identity and persistence | v13 forward migration, literal builtin plugin, trusted `agy` launch | Go schema/domain/launcher tests; FK check; preexisting identities unchanged |
| Headless transport | Native Task Executor and model inspector routed only for `antigravity` | Structured stream, failures, quota, cancellation, ToolSupply denial tests |
| User-facing surface | OpenAPI SDK, Wails, React, i18n and CLI | Typecheck, marketplace and creation, i18n tests |
| Integration | Native no-tool smoke, full `pnpm check`, evidence | Measured result; no production DB changes; clean tracked git status |

## Discoveries

- Installed binary: `/Users/mini/.local/bin/agy` v1.4.2, authenticated interactively by the owner (not probed or copied by Mender).
- `agy models` in v1.4.2 outputs `<slug>\t<display name>`; `models --output-format json` is **not** supported by this binary despite newer documentation describing such a flag.
- The CLI exposes `--input-format stream-json`/`--output-format stream-json`. No `agy acp` subcommand.
- Third-party `agy-acp` adapters are not used; they differ from the official headless interface and would add an installation and protocol review surface.

## Evidence and outstanding gates

Pending implementation and verification. No production Agent or credential changes authorized by this phase.
