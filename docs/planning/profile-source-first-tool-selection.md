# Profile source-first tool selection — focused delivery

Extend [ADR-0056](../architecture/adr/0056-source-first-mcp-tool-discovery.md) to the existing Profile configuration workflow. This is a UI hard cut, not a new Phase, data model, or execution route.

## A. Contract and read projection

- Providers/list gains optional read-only `toolSourceSelectable`: exact verified ownership and known Host source-wide availability (same semantics as MCP discovery). Source candidate picker requires `true` and `toolCount > 0`. Unclaimed and stopped Workspace facades are not addable, while already-bound tools remain manageable.
- Keep canonical ProviderID as source identity; two agent instances remain separate even when tool names/backend match. Jobs and Workspace suites of one Sandbox remain distinct source cards.
- No persistent schema migration, activation, Agent loop, Approval/Policy logic or production Profile migration.

## B. Two-level chooser and supplied sources

- Profile Tools view initially shows **selected sources** with count; click source to inspect/configure its bound tools and existing governance/presentation/credential actions. Preserve ToolID-level metadata; avoid repeating source name in every row. Missing selected tool metadata is hydrated by existing per-Tool detail API, but failures leave bindings intact and visible as unknown.
- Choose tools opens **selectable sources** first, with name search and current selected/available counts; enter a source to browse/search only its tools with Catalog cursor paging. Checkboxes and Hybrid Direct flags remain per ToolID. Explicit select-loaded acts only on loaded matching ToolIDs, no future auto-grant.
- Switching sources, filtering and paging do not clear unsaved selections. Saving uses existing atomic replace and retains full existing Binding overrides/credential constraints. Cancelling makes no change. One modal visible at a time.

## C. Validation and commits

- Test source-first entry and drill-down, unclaimed/unavailable exclusion, multi-source edits and cancel, selected metadata >100, provider-scoped pagination and search, explicit select-loaded semantics, exact ProviderID on same-name sources, and conservation of alias/policy/credential/Direct flags.
- Run relevant Vitest, TypeScript, Go host/API tests, and complete `pnpm check` including Race Detector and Wails/SKD checks. Commit docs, host/API projection, frontend UI/tests, and evidence separately. Do not restart running Wails/Tunnel or mutate production SQLite. Keep `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/` untouched.
