# Mender Brand Visual Convergence — Acceptance Record (2026-10-19)

## Goal / source of truth

- Product rules: `docs/design/mender-visual-identity.md`.
- Scope and ownership: `docs/design/brand-page-convergence.md`.
- Visual reality checked against the user's 2026-10-19 Wails Overview screenshot (1280-ish desktop window). The application is the existing Mender management product, not a standalone prototype.
- No backend, SDK, Agent state transitions, sandbox execution, tool permissions, persistence or tunneling behavior changed.

## Source-first implementation

| Layer | Changes |
| --- | --- |
| Shared UI | Semantic status Badge variants (positive, information, caution, negative, neutral) with explicit 'unknown' preserved; existing unstyled statuses remain neutral |
| Shared workbench | Overview KPI strip uses its **own available width** (@container); 1280px overview displays one four-item row, 390px uses a single readable column. Compact metric and panel typography |
| Shared controls | Tabs, filters, actions naturally cohabit the toolbar when there is room, wrap under constrained width; filter popover and guidance banner use semantic brand tokens |
| Overview | Existing activity/resource switch and preview-first behavior retained; audit status badges use common semantic presentation |
| Tools | Real Provider-card and Catalog-table layout updated, not an extra wrapper. Focus, long labels, card actions, small-screen table minimum width and scroll refined |
| Marketplace | AGY: card/filter/details/hover and accessible focus patterns; degraded is **attention** rather than terminal failure |
| Skills | AGY + review: badges, typography, mobile card content above actions; accessible action buttons. A long description is visually limited to 3 lines and available through detail/tooltip |
| Agents / Sandboxes | Read-only status badges, activity records, task uncertainty preserved; small-screen microtype made readable |
| Governance | Audit/Policy/Server/Runtime status semantics and supporting typography harmonized |
| Tunnels | AGY + review: better long-ID tables, semantic badges, responsive controls, modal text and selection focus. Reverted inert one-choice tab; retained existing configured-count copy |
| Profiles / Credentials / Other routes | Inherit shared shell, typography, cards, dialogs and toolbar standards. Desktop/narrow-width browser smoke checks completed; no feature-specific business change |

## Delegated AGY implementation (two real tasks)

- `atask_3a54123c2d85bea94a318976e8b3653f`: **completed**; only edited Plugins + Skills presentation. Reviewed before integration. Task output read from Mender's recorded task lifecycle.
- `atask_38d67e49741f1a926f2c71d4a1c8cf11`: **completed**; edited Tools + Tunnels. Reviewer **discarded** the Tools wrapper-only modification and changed the real underlying Provider/Catalog components instead, and removed a one-choice tab with inert click. The Tunnels presentation refinements were retained.
- Neither AGY task committed files or edited backend/runtime. Original TaskIDs were tracked; no automatic replay or duplicate delegation.

## Automated acceptance

- `pnpm typecheck`: passed for UI, SDK, App, Web and Wails frontend.
- `pnpm lint`: passed.
- `pnpm i18n:check`: passed: **1395 / 1395** source and zh-CN keys.
- `pnpm test`: **48 files / 288 tests passed**. A Server Degraded visual test was updated to explicitly check the brand-semantic caution color while still asserting that polling follows the actual lifecycle. No runtime state behavior changed.
- `pnpm build`: Web + Wails frontend success.
- Build reports JS bundle >500KB warning. This is a non-blocking **performance/code-splitting follow-up**, not part of the branding/UI-only scope.

## Browser QA

Chromium 148 CDP, real production Vite Web bundle served by a temporary **QA-only static/API proxy**, with the API pointed at an isolated local `menderd` on `127.0.0.1:18080` — **not the production Wails app state**.

| Viewport | Checked | Results |
| --- | --- | --- |
| 1280×800 | Overview | 4 KPI columns rendered in a single row; sidebar collapsed 258px → 80px and expanded back; quick navigation opened and closed from the header |
| 1440×900 | 15 management routes | No page-level horizontal overflow or JS exceptions; pages include Marketplace, Tools, Skills, Agents, Sandboxes, Profiles, Runtime, Servers, Tunnels, Policy, Credentials, Audit, Agent creation, Sandbox creation and External Sandbox creation |
| 390×844 | 10 routes | No page-level horizontal overflow or JS exceptions; accessible mobile navigation remains functional |
| 390×844 dark | Overview | Dark palette renders and no horizontal overflow |
| **All** | **27 route/viewport combinations** | No JS runtime exceptions or document-width overflow detected |

Screenshots and machine-readable route measurements remain on the Mac in `/tmp/mender-brand-convergence-qa/`. Key captures: `01-overview-1280.png`, `03-tools-1440.png`, `04-skills-390.png`, `05-overview-390-dark.png`. These are **working QA artifacts** rather than production assets.

### Explicit synthetic-data verification

The isolated local `menderd` was using `/tmp/mender-redesign2-state/mender.db` which lacked `skill_installations` and `skill_deployments` tables, so that development HTTP API returned 500 for Skills. Browser screenshots of those errors do **not** count as real populated-page acceptance.

For visual coverage only, a separate CDP request interception supplied two schema-conforming **read-only Skill fixtures** without altering Mender's database or code. At **390/768/1440** widths respectively, the Skill item measured `flex-direction: column/column/row`; all three had document width equal to viewport width and zero JavaScript exceptions. The 390px screenshot shows the corrected content-over-actions layout. Fixture presence is not represented as real user data.

### Known limits

- Several pages in the isolated test database show `Mender API 请求失败` due to missing or outdated database tables. Visual shell and error/empty states were inspected but real populated lists, create/edit dialogs and workflows were **not** end-to-end approved from these views.
- The running Wails application was **not rebuilt/restarted** automatically. The Wails **frontend production build** passed, but live desktop appearance and platform-specific behavior require the user to rebuild/restart and supply a screenshot/allow live inspection.
- Browser QA found no horizontal page overflow or JS exceptions, but this does not assert exhaustive accessibility compliance or UX completeness for every dialog state.

## Verdict

**Source UI convergence:** implemented for shared foundations and representative domain-specific surfaces, with all main routes smoke-checked. **Automated build/tests: PASSED. Browser shell/responsive QA: PASSED. Populated operational data and live Wails QA: PARTIALLY VERIFIED / needs full running environment.** Do not claim an exhaustive feature-by-feature sign-off until those blockers are resolved. Avoid altering database schema, production credentials or Agent lifecycle as part of visual remediation.
