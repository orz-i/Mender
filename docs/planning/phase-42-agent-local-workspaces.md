# Phase 42 — Local Agent workspaces without Sandbox coupling

- Status: active (2026-10-16).
- Authority: [ADR-0049](../architecture/adr/0049-agent-owned-local-workspace-paths.md).

## Intent contract

**User outcome:** Selecting a local project directory for an existing Agent is as easy as selecting its first `workspaceRoot` on Agent creation. Existing Sandbox resources are optional, irrelevant to Agent cwd selection, and remain independently governed when exposed as Workspace Tools. A user entering `GAR` sees the normalized alias `gar` and a directly choosable local directory. No duplicate Agent or tool creation.

**Hard boundaries:** Only owner-selected validated paths are persisted, runtime model arguments name only an approved alias, Profile workspace selection requires a separate explicit grant, and the admitted Task's key and root must be verified in the same SQL admission decision. Never use raw Tool cwd, Sandbox-derived implicit Profile grants, remote CLI fallback or compatibility bridges.

**Scope:** ADR/docs; SQLite v9 migration with owner/isolation/readiness-safe legacy mapping; Agent application and admission; HTTP/OpenAPI/SDK/Wails/CLI; one focused Workspaces dialog; regression and native smoke tests. Do not commit files under `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/`. Do not modify the user's live Agent, Profile, Sandbox or Tool inventory during regression tests.

## Commit boundaries

1. **A — Docs only:** ADR-0049, Phase 42 intent and acceptance (commit first).
2. **B — Domain / store / application:** explicit local root, migration v9, direct owner registration, per-Task exact target/authorization verification (commit after Go tests).
3. **C — Surfaces:** remove `sandboxId` from Agent workspace contract, update management API/CLI/OpenAPI/SDK/Wails, direct path selector and alias validation, refreshed i18n and UI tests (commit after parity).
4. **D — Acceptance/closure:** isolated real HTTP and MCP tests, v8→v9 snapshot/identity comparison, full `pnpm check`, Wails native rebuild smoke only when live tasks/leases zero, documentation (final commit). No real billable model request for this change.

**Potential risk:** legacy workspaces may refer to stale/unavailable Sandboxes; never silently resurrect their root or discard Profile grants. Fail migration with an actionable validation error instead of deleting or reassigning them. Historical Task snapshots are unaffected by binding migration.
