# Phase 46 — Optional governed tool execution settings

Status: owner-approved, work in progress, 2026-10-17. Authority: [ADR-0052](../architecture/adr/0052-optional-agent-governed-tool-placement.md).

## Fixed stages

| Stage | Deliverable | Gate |
|---|---|---|
| A · Documents first | ADR-0052 and concrete UI/runtime/rollback contract | Docs commit |
| B · Persist exact owner placement | Agent schema v11, atomically saved/default-or-alias root, SandboxID and existing selected ToolIDs, preserve history/identities | v10 fixture migration, duplicate/invalid settings and owner-only tests |
| C · Runtime supply wiring | Inject optional TaskToolSupply after Task admission, always close resources; recheck placement and all grants per operation | isolated Runtime + synthetic ACP real MCP read/edit, revoked/changed selection tests |
| D · Wails focused UI + acceptance | Agent detail tool execution tab, configure dialog (matching Sandbox and eligible tools), disable action; no remote HTTP approval/query | React/Wails unit tests, local safe fixture, `pnpm check` and migration fingerprints |

## Hard limitations

No currently installed Agent is modified automatically. The registered Agent workspace cwd stays independent of Sandbox, while governed **execution placement** optionally references a matching ready restricted local Sandbox. Host Agent cannot be configured for protected mode without an explicit eligible Agent resource; no hidden isolation upgrades. If a backend does not advertise ACP v1 HTTP MCP or has no viable credentials inside the limited process, it fails closed. Do not copy credentials or loosen Seatbelt to force success.

Production live Agent SQLite remains untouched by tests. Continue to preserve all unrelated `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/`. Code remains DDD/Clean Architecture: domain placement and owner service port independent of Agent protocol, bootstrap composes canonical governance capabilities, ACP adapter handles protocol conversion only. This phase must not become a new Agent Harness.
