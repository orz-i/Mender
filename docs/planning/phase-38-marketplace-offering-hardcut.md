# Phase 38 — Plugin Marketplace Offering boundary hard cut

Status: **Closed for automated acceptance (2026-10-15)**; packaged Desktop/live-runtime visual acceptance remains operator-only.
Architecture: [ADR-0045](../architecture/adr/0045-marketplace-offering-eligibility.md)

## Intent and invariants

**Outcome:** Discovery shows actionable MCP, OpenAPI, Model Tools, reviewed MCP integrations and healthy typed resource drivers. Internal resource-owned Tool Suites/Provider facades, roadmap placeholders and unsupported/unavailable entries no longer impersonate usable Marketplace products. Exact native installation management stays reachable even for disabled/degraded packages.

**Must preserve:** ProviderID/ToolID/SourceKey, configured Agent/Sandbox/Tunnel identities, exact installation pin, Credential references, Profile grants, Policy, audit, admission/lease/governance, CLI/HTTP/SDK/Wails parity, user-owned resources and data. Catalogue queries remain process-free; native package installation remains local Desktop-only. No Agent loop, no compatibility bridges, no extra plugin protocol, no ongoing dual-read inventory. Never stage `.agents/`, `.pi/`, `.intent-loop/`.

**Acceptance:**

- SC1: Host-controlled eligibility is explicit and fail-closed; unreviewed built-ins, unverified external installs and nonexecutable extension kinds cannot produce user create actions.
- SC2: Marketplace default cards: healthy reviewed Provider/Sandbox/Agent/Tunnel extensions, including MCP, Model Tools, OpenAPI, plus reviewed integration recipes. No internal facades, Tool Suites, E2B or unavailable-as-runnable cards.
- SC3: Native package inventory stays managed as exact installations; disabled/invalid/degraded instances remain visible and enforce guarded state changes/removal. No untrusted Web path install.
- SC4: Provider creation picker contains only independent, user-configurable Provider types. Agent/Sandbox Tool Suites continue to be prepared and repaired on their owners; no automated Profile exposure.
- SC5: End-to-end links open focused forms and preserve plugin and installation IDs; Go/TS tests, generation drift, lint, i18n, build/security gates pass. Existing users' resources are not touched by tests.

## Bounded implementation and commits

- **A — document-first decision:** `c0ffbc4` — ADR-0045, this contract, ADR-0039 presentation amendment, historical Marketplace document.
- **B/C — Host-reviewed projection, contracts and focused UX:** `bdd041e` — typed `marketplaceAction` shared across HTTP/OpenAPI/SDK/Wails, remove synthetic E2B technical item, default Discover/Installed separation, restore MCP/OpenAPI/Model Tools cards, retire Tool Suite cards and Foundation Driver toggle, reject independent managed-facade Provider UI creation. Keep exact native installation diagnostics and source/binding IDs.
- **D1 — lint closure:** `34416e6` — correct test-only shell invocation gosec annotations and remove obsolete E2B test counter; Go lint remains fully enabled.
- **D2 — HTTP contract regression closure:** `fbeceb0` — HTTP catalogue asserts Host action classification, correct first-party create eligibility, internal facade none, and no phantom E2B.

## Verification evidence

- Targeted React/desktop contracts: **4 test files / 77 tests passed**.
- Full frontend suite: `pnpm test` **40 test files / 217 tests passed**, with **1216/1216** Chinese locale messages.
- `pnpm lint`, `pnpm typecheck`: **PASS**, including Web and Desktop frontend.
- `pnpm build`: **PASS** for Web and Desktop frontend. Existing non-blocking Vite warnings about JavaScript chunks over 500 kB remain.
- `pnpm go:lint`: **PASS**, zero issues.
- `go test ./backend/internal/bootstrap ./backend/internal/desktop/binding -count=1`: **PASS**.
- `go test ./backend/internal/management/httpapi -run '^TestUnifiedPluginCatalogueManagementContract$' -count=1`: **PASS**.
- Combined `pnpm check` covered plugin SDK/example lint/test/race/build, Go module isolation, SDK schema generation drift, Wails binding drift, Desktop architecture, Go lint, all `go test ./backend/...` and `go test -race ./backend/...` without reported failures, then the remote tool request ended during the `eslint apps packages` phase. Subsequent standalone `pnpm lint`, `pnpm typecheck`, `pnpm test`, `pnpm build` **all passed**. Do not claim the monolithic command itself returned zero.
- `pnpm security:check`: **PASS**. No reachable Go vulnerability found; an uncalled dependency-module finding was reported, and production JavaScript audit reported no known vulnerabilities.
- `git diff --check`, explicit staged paths, generated contract parity, clean tracked worktree verified. Pre-existing untracked `.agents/`, `.pi/` and `.intent-loop/` were excluded from every commit.

## Deliberate acceptance limits

No real user Provider, Sandbox, Agent, Tunnel, Profile, Credential or package installation was modified for acceptance. Packaged Wails visual navigation and live third-party/remote service checks were not claimed as complete. Synthetic test fixtures do not establish that external remote services are reachable. No data migration is needed: this is a projection/entrypoint hard cut, preserving canonical resource and installation identities.
