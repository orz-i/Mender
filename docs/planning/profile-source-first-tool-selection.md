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

## D. Completed implementation and verification — 2026-10-18

- `62ec250` — ADR-0056 extension and this source-first Profile UI contract (documentation first).
- `17a5245` — added optional `toolSourceSelectable` to the existing HTTP `Provider` projection, OpenAPI contract and generated SDK, using the exact verified Host ownership + existing cached source-wide readiness. Existing Ready→Stopped→Ready Workspace HTTP test passes.
- `7fa0c50` — reused the same read-only Host projection without N+1 Provider lookups; Wails `Provider` model, generated bindings and desktop TypeScript adapter now carry exact `toolSourceSelectable`, with native adapter tests preserving `true/false` and the existing source owner identity. No second Source registry, lifecycle probe or activation.
- `022df74` — replaced the Profile flat chooser with dedicated `profile-tool-source-picker.tsx`: **selectable sources** first (exact ProviderID), then scoped Catalog Tool browse/search/cursor and individual ToolID selection. Existing selection, Hybrid Direct flag and all Binding overrides survive navigation and atomic `replaceProfileTools`. Explicit **Select loaded tools** never includes unloaded pages/future tools. Avoid no-op saves/revisions. Profile management also lists **selected sources** before per-tool Presentation/Governance/Credentials/Removal. Existing unknown/unavailable bound ToolIDs remain visible for repair; missing Catalog metadata is hydrated by exact ToolID and read failures do not erase selection. Same-name source instances are distinguished with their unique Provider namespace only when names collide, using memoized source labels. Replaced dead picker UI and retired its translations; i18n now has 1368 required/1368 zh-CN messages.
- `531e010` — updated the full Profile management integration test to the new source→tools flow, retaining its original Alias, Policy, Concurrency, Credential and one-modal invariants.

### Evidence

- The previously flat Profile-specific UI scale tests were rewritten for source-first and extended: multiple sources, 105-tool backend paging, selected ToolID preservation beyond the initial Catalog 100, explicit select-loaded, no-op save, same-name Agent instances, cancel during pending async load, historical ToolID with failed metadata lookup, and all existing ToolBinding overrides. Desktop adapter tests prove valid true/false propagation. All relevant Go packages and targeted Vitest/TypeScript/ESLint checks passed.
- First full `pnpm check` exposed an **old Profile management integration test** still relying on flat `Choose tools` behavior and a separately failing Marketplace Echo test. The Profile test was updated with the two-step source drill-down and kept all authorization assertions. The Marketplace test passed isolated and then in the whole test file without app code changes. The final complete `pnpm check` then **passed, exit 0**: `/tmp/mender-profile-source-check-final-20261018.log`. It includes Go tests plus Race Detector, golangci-lint `0 issues`, Provider/Sandbox/Tunnel SDK/example checks, SDK generation and Wails binding checks, ESLint and TypeScript, i18n (1368 strings), **47 Vitest files/286 tests**, and Web/Desktop builds. Existing nonfatal Vite large-chunk warnings remain.
- Production SQLite was inspected **read-only** after implementation: `default` Profile still has revision `7` and 29 stored ToolBindings. No migration, mass grant, Profile mutation, approval/trust change or production DB write occurred. The running Wails/Tunnel remained untouched and the compiled desktop binary had its previous timestamp; **the newly built source-first Profile UI has not been deployed or visually accepted yet**. Owner rebuild and refresh, then UI acceptance should confirm the dialog's source list, underlying tools, existing unavailable bindings, cross-source draft selections, and that one save retains 29 existing bindings except explicit user changes.
- The four pre-existing untracked folders `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/` were neither cleaned nor committed. The local branch remains unpushed.
