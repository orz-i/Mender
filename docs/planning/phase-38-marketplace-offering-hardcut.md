# Phase 38 — Plugin Marketplace Offering boundary hard cut

Status: In progress · 2026-10-15
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

## Bounded commits and checks

### A — Docs-only decision (must commit first)

Record ADR-0045, this acceptance contract and superseding Marketplace product wording. Check git diff and staging scope; no code modifications.

### B — Host-owned projection and contracts

Add action eligibility derived solely from reviewed built-in IDs and supported verified native typed extensions. Apply it identically to HTTP/OpenAPI/SDK and Wails/Desktop. Retire E2B as a synthetic installed Marketplace catalogue entry while retaining the backend's nonexecutability safeguards and an optional Sandbox-specific roadmap reference. Add tests for internal/unknown/unavailable/installed cases. Commit independently.

### C — Focused UI and owned create workflows

Use only the host-declared action. Default discovery shows actionable entries plus curated integration recipes; split installed package management from discovery with an existing focused switch/card grid, not duplicate inventories. Remove Foundation Driver toggle, Tool Suite card transformation and manual internal Provider chooser eligibility. Keep focused create flows, exact pinned installation links, resource-owned Suite views and i18n. Update tests and commit independently.

### D — Closure

Run deterministic Go/TS tests, full `pnpm check`, `pnpm security:check`, drift/build checks, `git diff --check` and review final staged/unstaged scope. Fix actual regressions without weakening checks; commit bounded fixes if needed. Desktop live UI smoke is separate if no active running Desktop/UI availability can be verified.

## Evidence

(To be completed from actual commands and commits; no pass claimed in advance.)
