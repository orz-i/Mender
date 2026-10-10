# Page-level toolbars: route-wide QA and sign-off

**Date:** 2026-10-10
**Scope:** shared Mender Web/Desktop management UI
**Result:** page-level toolbar hierarchy and results-only inventory surfaces **verified** at tested widths; whole-product pixel fidelity to every supplied Elera screen remains outside this scoped sign-off.

## Change
The management shell already supplies the page title. Inventory category Tabs, searches, filters, sorting, and Refresh/New/Install actions now share one **page-level toolbar outside result cards**. The result card shows only its inventory/grid/table, loading or empty state and pagination. It retains an sr-only section title for assistive technology.

Shared implementation:
- `packages/app/src/shared/page-toolbar.tsx`: `PageToolbar`, `PageTabs`, `PageTab`, `ResultPanel`.
- No backend/contract/provider/Principal/security-policy changes and no dependency additions.

## Route migration matrix

| Route | Page-level controls | Results / details |
| --- | --- | --- |
| `/skills` | All/Built-in/Installed/Enabled + search/source/status/sort + Refresh/Install | one results-only Skill Library card |
| `/plugins` | plugin search/type/source/status + Refresh/available installer actions | card grid with cursor pagination |
| `/providers` | safe local provider search + Refresh/Add provider | provider table and pagination, create/edit dialogs unchanged |
| `/catalog` | intent search/provider filter + Search/Browse | tool results table and pagination |
| `/profiles` | safe local profile search + Refresh/Create profile | profile table and pagination, manage dialogs unchanged |
| `/credentials` | safe inventory-metadata search + Refresh/Add credential | credential table and pagination, secrets never queried by the search |
| `/policy` | default decision preview + Edit/Add rule | policy rules table and pagination |
| `/servers` | safe local search + Refresh/Start server | server table and pagination |
| `/tunnels` | safe local search + on-demand Tunnel information/Add tunnel | tunnel table and pagination; large explanation moved to focused dialog |
| `/sandboxes` | category Tabs (All/Ready/Local/Unavailable) + search/backend + Create | sandbox table and pagination; previous redundant metric tiles become actionable category counts |
| `/audit` | Invocation/Provider tabs + search/status + Refresh | one active table and pagination; details unchanged |
| `/runtime` | Runtime/budget/isolation switch and contextual Edit | focused status/preview; editor dialog preserved |
| `/` | Activity/Resources switch | existing metrics and focused dashboard previews retained |
| `/sandboxes/:id/activity` | event kind filter and Back action | event table/pagination |
| `/plugins/:pluginKey` | details section switch + Back/Add/activation controls | one focused detail panel |
| `/sandboxes/:id` | overview/security switch + Back/Activity/Manage | focused details and manage dialog |
| `/sandboxes/new` | Back outside wizard card | staged create wizard; within-step Next/Review/Submit remain inside |

## Real Chromium layout check

The QA run used a headless Chromium CDP browser against the existing development Web app at `http://127.0.0.1:5175`. It inspected real DOM rectangles, screenshots and script exceptions; no fake UI was generated for this work.

- **13 main routes** inspected at both 1440×900 and 390×844 CSS pixels, plus plugin detail and Sandbox create/detail/activity routes.
- At 1440×900, Skills tabs `y=96..140`, filters `y=100..136`, actions `y=102..134`: **same toolbar row**. `ResultPanel` is a separate DOM node after the toolbar; content not nested inside the toolbar.
- **All inspected 390px routes:** `document.documentElement.scrollWidth === innerWidth === 390`. Filters and action groups wrap below tabs as needed, instead of compressing search to unusable widths.
- **11 inventory routes** mount one `ResultPanel`; Overview and Runtime remain focused dashboard/detail compositions. Plugin item cards correctly retain their own per-item headings; inventory headers no longer repeat as visible card titles.
- Tested routes emitted **no observed JavaScript runtime exceptions**.
- Screenshots on the QA Mac:
  - `/tmp/mender-toolbar-skills-1440.png`
  - `/tmp/mender-toolbar-skills-390.png`
  - `/tmp/mender-toolbar-plugins-1440.png`
  - `/tmp/mender-toolbar-audit-1440.png`
  - `/tmp/mender-toolbar-detail-io.mender.provider.mcp%40builtin-1440.png`
  - `/tmp/mender-toolbar-detail-new-1440.png`

**QA limitations:** the existing 5175 dev backend is older than the current UI in some resource endpoints; some mobile screenshots show a genuine API error notice because an endpoint is unavailable. This is not a successful backend integration claim. Test-only installer availability varies with the Web client; the Skills React integration test additionally verifies `Install Skill` is rendered in the external toolbar when available. Sandbox detail data was unavailable on the old QA server, so those routes were inspected for toolbar boundaries only; prior focused Sandboxes testing covered supported lifecycle behavior. Not all dark-mode/OS viewport combinations have been pixel compared.

## Functional and automated checks

- **`pnpm check` passed (exit code 0)**, including Go/module/race/plugin and generated contracts, ESLint, TypeScript, Vitest, and both production frontend builds.
- **17 test files / 100 tests passed**, including new shared PageToolbar category switching, semantic section ownership, result-panel separation and Skills install placement.
- i18n: **874/874** required Chinese translations.
- Existing pagination, cursor histories, search behavior, detail dialogs, create/edit actions, and permission gates were retained.
- `git diff --check` clean.

## Non-goals
No redesign of provider/sandbox execution, no new Agent loop, no new compatibility bridge. This stage specifically targets consistent **layout hierarchy** rather than replacing real Mender content with sample Elera healthcare data.
