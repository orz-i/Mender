# Phase 39 — Tools workspace and canonical source projection

Status: Native Wails dev-mode acceptance closed · 2026-10-16
Architecture: [ADR-0046](../architecture/adr/0046-unified-tools-workspace.md)

## Scope and constraints

Implement **Option B** from the Provider/Catalog audit: unify the *product* entry and keep independent Provider and Catalog bounded contexts. No database reset, no new Source/Plugin inventory, no compatibility routes, no Agent loop. Keep stable ProviderID, ToolID, SourceKey, package installation pins, Credential References, existing Agent/Sandbox Tool Suite bindings, Profile ToolBindings, Policy, grants, audit and execution guards. Do not stage `.agents/`, `.pi/`, `.intent-loop/`.

## Bounded commits

- **A — Docs-first (commit before code).** ADR-0046, this plan, route/product language, schema and acceptance invariants.
- **B — Tools entry and focused create workflow.** Stage B (`36fe731`) initially introduced a `catalog/sources` tab. The approved refinement replaces that intermediate product UI: `/tools` shows unified named service cards (AnySearch), `/tools/:providerId` shows *that service's Tools*. Remove the temporary tab and its query parameter; no old `/providers`/`/catalog` pages. Marketplace Connect/Create opens a focused form through `/tools?create=1&offeringId=…` or `pluginId=…`; resource-owned entries have owner detail navigation. Do not restore duplicate Provider plugin picker.
- **C — Source attribution.** Stage C (`0bd3e4a`) supplies Host-owned read-only attribution through exact existing Agent/Sandbox binding records (HTTP/SDK/Wails). Every service appears *uniformly* as an entry, never as an internal/external category. User-visible AnySearch instance name and Tool count take priority over underlying MCP/Provider metadata; owned resource Manage navigates to Agent/Sandbox. Unclaimed facade remains neutral and diagnostic. No guessed association from PluginID.
- **D — Service drilldown and complete paged discovery.** Service cards → exact source Tool list, with searched/browsed results strictly scoped to its ProviderID. Bounded cursor pagination in canonical browse and FTS search, server-side Provider/Profile scope, deterministic tie breaks, invalid/stale cursor failure. Profile Tool Picker uses the same paged backend rather than filtering the first 100 local results. No full schemas on list, no auto Profile grant.
- **E — Acceptance and cleanup.** Focused Go/TS contracts; `pnpm check`, `pnpm security:check`, generated bindings, isolated actual UI; ensure no unintended user-data changes, remove obsolete code/wording and document final evidence. Close with bounded commits.

## Safety / release

Every schema/API shape update must compile across OpenAPI generated TS, HTTP API, CLI and Wails bindings. User data are only read in acceptance; test writes use disposable SQLite. A running Wails app must be rebuilt/restarted by the operator for new source to take effect. The previous routes are hard-retired, not redirected to an alternative implementation. If an unavoidable migration is identified, fail closed with a documented recovery path rather than automatically reassigning IDs.

## Acceptance evidence (2026-10-16)

**Commit sequence (bounded):**
- `0cf35a8` — ADR and first product/ownership intent.
- `36fe731` — hard cut of standalone Provider/Catalog routes and redundant Provider plugin picker (the interim two-tab UI was later retired, not preserved as a bridge).
- `0bd3e4a` — exact persisted Agent/Sandbox owner projection, parity across HTTP/OpenAPI/SDK/Wails.
- `9aa92c2` — user-driven refinement to a simpler **named service → its tools** product model.
- `95d741d` — `/tools` service cards and `/tools/:providerId` drilldown; removes internal/external filter, SDK-provider-first labels and temporary dual-view switch.
- `505508d` — invariant for revision-scoped opaque cursors.
- `c0f55d0` — transactional `catalog_tool_revision` migration, bounded compact browse/FTS paging with exact Profile restrictions, HTTP/SDK/Wails parity, Profile picker load-more without losing prior bindings.

**Automated checks:** A fresh `pnpm check` returned **exit code 0** (log `/tmp/mender-phase39-final/pnpm-check.log`), including Plugin SDK/examples and module isolation, OpenAPI/SDK generated-schema drift, Wails generated-binding drift, desktop architecture, Go lint and full Go test/race, ESLint/TS/i18n, **41 frontend test files / 227 tests passed**, and Web/Desktop production builds. **1200/1200** required zh-CN translations passed. Vite warns about large JS chunks; no build failure. Catalog/HTTP pager integration tests covered **205–253+ Tools**, verified no duplicates/gaps, deterministic scoping, invalid/cross-provider/cross-Profile cursor rejection and revision invalidation. Profile picker tests covered Tool 105 across pages and preserved existing ToolIDs.

**Security:** A fresh `pnpm security:check` returned **exit code 0** (log `/tmp/mender-phase39-final/security-confirm.log`): govulncheck found no reachable vulnerable Go symbols in the backend, Desktop, Plugin SDKs or examples; it noted one required-module vulnerability which these applications do not appear to call. The production JavaScript audit found no known vulnerabilities.

**Isolated runtime acceptance:** A separate source-built Go `menderd` at `127.0.0.1:18085` used exclusively disposable database `/tmp/mender-phase39-qa/qa.db`; a local in-process MCP fixture at `127.0.0.1:18232` returned `web_search` and `page_lookup`. It created exactly one configured MCP Provider **displayed as AnySearch**, synchronized two canonical ToolIDs. Chromium testing of the Web frontend at `127.0.0.1:5182` verified **one actual AnySearch link**, its **2 tools** count, scoped `/tools/:providerId` tool list, Back to services, absence of the technical Provider/internal/external toggle, and the reviewed AnySearch Marketplace Connect dialog with its preconfigured HTTPS endpoint. No JavaScript runtime exceptions. Screenshot/log evidence under `/tmp/mender-phase39-qa/`. All four dedicated QA listeners (MCP fixture, menderd, Vite, Chromium) were terminated and verified closed, the temporary Vite config and disposable SQLite database were deleted, and the user-owned Wails process was left running. This confirms the product workflow **without contacting the actual AnySearch remote service**.

**Real user data safety:** Read-only check of `~/Library/Application Support/Mender/mender.db`: `PRAGMA integrity_check=ok`, 0 foreign-key violations. Live counts at check time: 9 configured Providers, 25 Tools, 6 Sandboxes, 2 Agents, 1 Profile, 21 Profile Tool bindings, 2 Credentials. No live Provider, Tool, Credential, Profile or Sandbox was created/removed or re-synced for acceptance. The earlier automated-only check preceded a Wails restart. Subsequent [native Wails acceptance](phase-39-wails-native-acceptance.md) verified Catalog v2 and all revision triggers in the live database after the operator's 16:01 rebuild/restart. Historical migration backups were not touched.

**Native follow-up and release boundary:** [Phase 39 Wails native acceptance](phase-39-wails-native-acceptance.md) verified the rebuilt macOS dev window, exact AnySearch Tool drilldown, reviewed Connect preview, Profile Tool Picker and Agent/Sandbox owner navigation. One Wails adapter owner-field omission was corrected in `89a5345` and full post-fix checks passed. Actual remote AnySearch service availability, Credential validity, tool execution, Agent backend execution and separately packaged release build were not verified. One pre-existing unclaimed Workspace Provider remains a documented data-ownership follow-up; no relationship was guessed or mutated.
