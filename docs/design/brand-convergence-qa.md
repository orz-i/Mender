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

## Native Wails real-data QA checkpoint — 2026-10-19, after owner rebuild

The **owner has rebuilt and restarted** the Wails dev client after the branding changes. Native app PID 23817 launched 2026-10-19 13:12 local time, desktop binary timestamp 13:12:07, after commit `cd2bdd7`. It serves its frontend on 127.0.0.1:9245 and runs the real owner database; this supersedes the earlier “Wails not rebuilt” limitation **for visual smoke testing**.

Without changing providers, tasks, policy, credentials or stored data, the coordinator used macOS WindowServer `screencapture -l 17986` and **native Accessibility AXLink/AXButton/AXCheckBox clicks** to inspect the real Wails window. Images remain on the Mac in `/tmp/mender-wails-real-qa/` (not repository assets). Verified live screenshots for **Overview, Plugin Marketplace, Tools, Skills, Agents, Sandboxes, Profiles, Runtime, Tunnels, Servers, Policy, Credentials, Audit and Approvals**. This is **14 distinct primary page surfaces including the Overview and Servers captures**, not just the earlier Vite mock. Real data were visible in Tool sources, Agents, Sandboxes, Profiles, Skills, Tunnel/Server inventories and Audit; controls did not require a frontend-only mock or migration of the isolated test DB. `PRAGMA quick_check=ok` on the live DB was read-only.

Opened **Antigravity Agent → read-only Overview → Tasks**, preserving the original historical `unknown` row without retry, recovery or reassignment. The Task result modal shows the non-authoritative/no-evidence warning correctly, but its header currently prints the raw English `unknown` status code; the Task list shows a translated caution badge. **UI-only discrepancy identified**: localize and apply the same semantic StatusBadge in modal header, improve long detail scrolling. Do not change Agent state, outcome, diagnosis, approvals, session recovery or backend inference.

Native Accessibility navigation subsequently became temporarily unavailable (AXWindow count 0) while the process and on-screen WindowServer image remained available. Do not infer a crash, restart the live app, or force foreground clicks based on that alone. Full dialog and resize matrix remains a separate acceptance item when native AX access resumes.

The following section is an implementation/acceptance addendum, not a claim of comprehensive workflow signoff.

### Native Wails read-only Task detail UI fix — completed

- Source commit `9533b21` changes only the Agent task **presentation and focused test**, without modifying the task state machine or native session recovery.
- Before: historical `unknown` was presented as a raw English code in `Agent task result` despite the correct localized caution badge in the list. After: task identifier and `状态未知` semantic badge are separate and clearly readable; the result dialog has a bounded scrolling body and persistent footer. The underlying no-evidence warning and diagnostic remain unchanged.
- Actual Wails window screenshots: `/tmp/mender-wails-real-qa/15-agent-unknown-detail.png` before and `/tmp/mender-wails-real-qa/16-agent-unknown-after-hmr.png` after. The latter **visibly shows** localized `状态未知` in the unchanged live Wails process; frontend Vite hot-reload sufficed, and no Agent task / database row was modified.
- Typecheck + ESLint + i18n **1395 / 1395**, focused Agent tests **23 passed**, complete Vitest **48 files / 289 tests passed**, Web and Wails frontend production builds passed. Existing nonfatal Vite bundle size warning persists.
- **QA limits remain:** the native Accessibility window disappeared from the AX tree during inspection, although WindowServer still returned the same on-screen Wails window and native process remained alive. This is an **automation inspection blocker**, not evidence of a Wails process crash. The isolated mock DB from prior QA remains unsuitable for real populated workflows; the current live Wails primary route images are authoritative for those visuals.

### Wails server Tunnel column correction — 2026-10-19

- User's real Wails screenshot showed a stacked display of Tunnel name `Mender`, separate `OpenAI` pill and `就绪` pill. The name / kind / readiness hierarchy consumed extra vertical space and presented the provider badge with the same weight as the actionable status.
- Source review found that `ServersPage` unconditionally rendered `OpenAI` even for `tunnelProvider="external"`, and defaulted a missing Tunnel name to `OpenAI Tunnel` regardless of provider. This was a **presentation error**, not a Tunnel transport or lifecycle defect.
- Fixed only the `ServersPage` read-only projection: primary line now shows **Tunnel display name + live readiness Badge**; a second muted text line shows `OpenAI` or `外部插件` according to the actual `tunnelProvider`, and omits it if metadata is unknown. Missing names get neutral `Tunnel` rather than an inferred provider. Long names truncate with native `title`; runtime connection and actions remain unchanged.
- Native Wails hot-reload verification: real Mender window navigated via macOS Accessibility to Servers; screenshot `/tmp/mender-servers-tunnel-cell-after.png` visually confirms `Mender + 就绪` above `OpenAI`. Real server was not restarted and no live configuration was changed.
- Focused test assertions cover both `openai` and `external` identity and absent provider metadata, as well as existing state polling. **Full UI quality gate** passed: TypeScript, ESLint, i18n **1394/1394**, **48 Vitest files / 292 tests**, Web and Wails frontend builds. Vite's prior nonfatal bundle-size warning remains.

## Verdict

**Source UI convergence:** implemented for shared foundations and major domain-specific screens. **Automated build/tests: PASSED (289/289). Browser shell/responsive QA: PASSED for tested routes. Real-data Wails native primary navigation and visible task-modal HMR: PASSED for inspected screens.** Full Wails dialog/create/edit journey coverage, keyboard and narrow-window platform testing remain **PARTIALLY VERIFIED** because native AX inspection became unavailable. Do not claim exhaustive feature-by-feature sign-off until those gates are run; do not change live database schema, credentials or Agent lifecycle merely to complete visual remediation.
