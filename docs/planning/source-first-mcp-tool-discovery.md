# Source-first MCP discovery — implementation and acceptance

Based on [ADR-0056](../architecture/adr/0056-source-first-mcp-tool-discovery.md). One focused implementation, no new Phase tree.

## A — contract (documentation commit)

- Freeze ProviderID as `sourceId`, verified exact ownership, source-first default, compact grouped search, opaque scoped cursor, Profile-first filtering and direct-tool presentation.
- Record old flat Meta Search response as deliberately retired; retain Describe/Call signatures.

## B — host projection (coordinator)

- Add bootstrap read-only resolver of `ProviderID -> sourceKind/sourceName/resourceId/providerName`, reuse existing exact owner/configuration query, and supply to MCP Adapter through a typed dependency.
- No new storage, duplicate source tables, activation, calls, or authorization shortcuts.
- Verify managed orphan/ambiguous bindings stay unclaimed/fail closed.

## C — search and pagination (OpenCode)

- Improve canonical FTS weighting/test quality in Catalog without treating provider namespace/plugin ID as owner.
- Add regression tests for lexical relevance, allowed-ID filtering, Provider restriction, >100 results, deterministic ordering and bad cursors.
- Independently verify query/pagination backward compatibility for non-MCP Catalog API.

## D — MCP source-first discovery (Antigravity)

- Implement the `view=sources|tools` Meta Search contract and grouped compact response, current source identity + effective alias/title/description matching, Profile allowlist before projection, cursor scope/staleness and tests.
- Qualify direct/hybrid exported human-readable metadata; keep names/Schema/ToolID and invocation untouched.
- Coordinate dependency interface with B; no edits to Catalog SQLite files.

## E — coordinator integration & evidence

- Inspect every diff and commit, run package tests then `pnpm check` including race detector.
- Non-destructive real tests: search OpenCode/Grok/Antigravity/Rocky/AnySearch and generic agent/workspace/read; same-backend ownership; Profile revoke/rename/hot refresh; describe/call correctly route ToolID. Do not restart Wails/Tunnel or mutate production without an explicit, narrowly scoped acceptance need.
- Preserve .agents/, .pi/, .intent-loop/, .commandcode/ and unrelated untracked files; segmented commits: docs, source projection, Catalog, MCP, integration/acceptance.

## Deferred finding — unknown Agent outcome after delegation timeout

- During this implementation the OpenCode and Antigravity delegated tasks changed from `running` to `unknown` with diagnostic `Agent outcome unknown after timeout`. A subsequent process snapshot no longer included their task-specific CLI processes, but neither the stored status nor a PID snapshot proves whether each task succeeded, failed, or was still executing in a detached subprocess. Actual repository edits survived.
- **Do not conflate timeout, process disappearance, cancellation, and final result.** Preserve the ambiguous `unknown` state rather than reporting success/failure or automatically resubmitting potentially non-idempotent work.
- Follow-up Agent lifecycle investigation (separate from this discovery change): record exact task/process/session ownership and last heartbeat, inspect whether backend adapters support reliable reconnection, reconcile process exit against durable task result, distinguish a live detached session from an irrecoverable outcome, and ensure generation fencing prevents stale writers after retry. Add crash/network-timeout recovery tests for OpenCode and Antigravity independently.
- This issue is **recorded, not fixed** here. Do not change existing permissions, approval, timeout or process orchestration behavior in the Source-first discovery work.

## Integrated implementation and verification — 2026-10-18

- **Commits:** `95e89db` architecture contract; `d553e5a` Catalog relevance + SQL-side allowlist; `88fff60` MCP source-first views + exact Host attribution; `300ad0d` deferred Agent timeout status issue. Removed the obsolete MCP `SearchTools` injected dependency; no Source table, persistent migration, authorization alteration or extra meta-tool entry.
- **Protocol tests:** genuine SDK MCP client sessions verify per-source identities for multiple same-backend Agents, duplicate `delegate` ToolIDs, Workspace name/alias/title override, verified Agent/Sandbox source ownership, unclaimed neutrality and hidden Profile tools; 137-tool pagination, cursor scope, stale catalog revision, renamed owner, invalid input and error propagation. Existing managed Streamable HTTP dynamic Profile hot-refresh test remains passing.
- **Quality gate:** complete `pnpm check` exited **0**, including Go test, Go race detector, golangci-lint (0 issues), SDK/provider examples, Wails bindings/architecture, ESLint, TypeScript, i18n (1350 keys), Vitest (46 files / 279 tests), and web/desktop builds. Vite emits only non-fatal frontend chunk-size advice. Git tracked tree is clean after commits.
- **Read-only live data cross-check:** existing configured Agent facades resolve to **OpenCode**, **Grok - executor** (although Provider display may say `Agent Delegation`) and **Antigravity** through exact `agent_tool_facades` relationships; Rocky is represented by separate Workspace/Jobs ProviderIDs. This is identity evidence from the current database, **not** an assertion that updated MCP code is running there.
- **Deployment acceptance outstanding:** the live Wails desktop process and Tunnel were deliberately left running unchanged. They were not rebuilt or reconnected during this work; the new ChatGPT-side `mender_search_tools` response has **not yet** been validated against a deployed build. After owner rebuild/reconnect, verify OpenCode/Grok/Antigravity/Rocky/AnySearch and agent/workspace/read queries, source selection + Describe/Call by exact ToolID, Profile revoke/add hot refresh and unchanged server/tunnel identity. Do not claim production deployment acceptance before that check.
- Both delegated Agent tasks eventually recorded `unknown` after timeout. Source edits were independently reviewed/tested; the ambiguous task outcome is tracked separately above, not represented as Agent-reported successful completion.

## Follow-up — deployment discovery admission, 2026-10-18

- Live ChatGPT MCP query confirms `Codex` yields zero sources. The `default` Profile contains **0** ToolBindings for Codex's four available Catalog Tools; the desktop **Choose Tools** dialog displays unselected candidates, not currently supplied tools. Do not remove Codex from the catalog or confuse the picker with MCP supply.
- Live `workspace` and `read` searches still exposed one `Unclaimed source` with four Profile-bound tools. Read-only DB inspection identifies a legacy `Mender` Workspace Provider with no verified Sandbox suite claim, pointing to an `Alpine` Sandbox whose persisted run is `stopped`. The existing Profile has 29 bindings; this source has four. No production data or grants are to be deleted or rewritten.
- Hard-cut discovery admission: exclude unclaimed/known-unavailable sources from `sources`, source-scoped `tools`, cross-source `groups`, and direct exported `tools/list`. Keep exact ToolIDs/Profile grants and the management diagnostic unchanged. Sandbox Workspace requires a Ready run and cached backend availability; Durable Jobs retain discoverability on Stopped because read-only recovery tools may still work. Do not initiate new health probes, backend activation or Policy changes in a discovery request.
- Add deterministic hidden/orphan/offline/Ready/Jobs-stopped tests and validate the deployed search after owner rebuild. Do not claim execution is guaranteed merely because a known source is discoverable.

### Follow-up implementation and quality evidence

- `832d6a7` updates ADR-0056 and this plan to explicitly exclude unclaimed/known source-wide unavailable Providers from MCP discovery. `888b212` implements Host-read-only Sandbox readiness projection, source-level discovery admission, the corresponding direct `tools/list` export gate, and current availability in the cursor fingerprint. It preserves existing Profile ToolBindings and ToolIDs, with no persistent Source registry or data mutation. `3512e8d` updates historical MCP integration fixtures to actually claim their Agent/Sandbox facade or configure their standalone provider; an unclaimed test facade is no longer treated as discoverable.
- New SDK MCP protocol tests verify default sources, source-scoped tools, cross-source grouping, explicit hidden sourceId, direct tools/list, unbound Agent visibility, unchanged Profile membership and invalidated cursor when availability changes. Host tests demonstrate Workspace Ready → Stopped → Ready filtering and preserve SSH Durable Jobs discoverability on Stopped for read-only recovery. Independent Catalog/profile/Agent ownership tests remain green.
- **Initial full `pnpm check`: FAILED** at the Go tests because five pre-existing fixtures used unconfigured/unclaimed providers while expecting direct export. **Corrective fixture changes:** configured the in-memory upstream MCP Provider tests, pinned Agent/Sandbox test facade ownership and explicitly configured the direct-name collision test source. The original invocation and authorization assertions were retained; all five failing tests passed when re-run.
- **Second full `pnpm check`: PASSED, exit code 0**, log `/tmp/mender-source-admission-check-rerun-20261018.log`. All Go tests including Race Detector, golangci-lint (0 issues), Provider/Sandbox/Tunnel SDK checks, generated SDK and Wails binding checks, ESLint, TypeScript, i18n (1350 strings), Vitest (46 files and 279 tests), and Web/Desktop builds passed. Vite chunk-size note is non-fatal.
- **Still requires a new owner rebuild/deployment:** the live Wails `mender-desktop` PID 15572 and Tunnel PID 16507 remained unchanged throughout this follow-up, so the currently running client has not loaded commits `832d6a7`/`888b212`/`3512e8d`. After rebuild/reconnection, verify query `Codex` is empty; query `workspace` includes only verified, available Rocky Workspace/Jobs where applicable; `Unclaimed source` is absent; exact hidden `sourceId` returns empty; and ToolID Describe/Call plus hot Profile refresh remain governed. No production database was changed and no Profile selections were removed.
- **Lifecycle nuance:** dynamic Meta Search reevaluates Host availability on each request; direct MCP `tools/list` is an adapter-build-time snapshot, so subsequent resource lifecycle changes can require an adapter rebuild to update direct exports. No new background watcher or unsolicited MCP notification was introduced.
