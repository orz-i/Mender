# Product Design QA — Command Workbench redesign

## Audit scope

- Selected direction: Product Design direction 2, **Command Workbench**.
- Product surface: Mender Web/Desktop shared management UI.
- Audit flow:
  1. Desktop Skills workspace.
  2. Skill focused detail Dialog.
  3. Catalog workspace.
  4. Mobile Skills workspace.
- Audit capture source: live local web app backed by an isolated temporary Mender Runtime.
- No production database was used.

## Current audit evidence

Accepted screenshots captured from the final working tree:

1. Desktop Skills — `/tmp/mender-audit-final-01-skills-desktop.png`
2. Skill detail Dialog — `/tmp/mender-audit-final-02-skill-dialog.png`
3. Catalog workspace — `/tmp/mender-audit-final-03-catalog.png`
4. Mobile Skills — `/tmp/mender-audit-final-04-skills-mobile.png`

Desktop viewport: 1536 × 1024 CSS px.

Mobile viewport: 390 × 844 CSS px.

The temporary Runtime contained the built-in `mender-tools` Skill and one trusted/enabled local `code-review` installation so that list, filters, and detail views were audited with real data rather than only empty states.

## Information architecture

The redesign now uses one shared page grammar:

~~~text
Page Header
  -> optional filter / configuration toolbar
  -> one primary data workspace
  -> focused layered Dialogs for deeper tasks

Overview is the only page that adds a metric/statistics strip before its primary data workspace.
~~~

This grammar is applied across:

- Overview
- Providers
- Skills
- Catalog
- Profiles
- Servers
- Policy
- Credentials
- Audit

The redesign intentionally reduces nested Card usage. Cards now communicate a meaningful grouping boundary instead of becoming the default wrapper for every section.

## Step audit

### 1. Desktop Skills workspace

**Health: passed**

Strengths:

- Sidebar, global page search, Runtime state, title/action area, filters, Skill list, and pagination have a clear top-to-bottom hierarchy.
- Skill statistics were removed from the domain page; aggregate statistics are reserved for Overview.
- Tabs and filters are grouped into one workbench toolbar instead of multiple visual containers.
- Built-in and local installations remain visually distinguishable without adding fabricated metadata.
- Source, status, search, and sort controls are functional.

Interaction evidence:

- The Skill source filter was opened in the live browser and switched from All Sources to Local.
- The built-in Skill disappeared and only `code-review` remained.

Accessibility observations:

- Desktop navigation links have explicit accessible names.
- Filter controls have explicit labels.
- Main icon actions retain tooltip/aria-label behavior.

### 2. Skill focused Dialog

**Health: passed**

The visual reference originally showed a persistent right-side inspector, but the product requirement explicitly overrides that interaction. Mender keeps the previously established focused Dialog model.

The default Skill Dialog shows one class of information:

- installation identity;
- source/version/digest;
- installed state/time;
- resource count/size;
- allowed-tools metadata.

It does not mix full resource inventory or deployment history into the same viewport.

Separate transitions remain available for:

- Skill resources;
- Skill deployments.

The Dialog uses a bounded viewport and internal scrolling, and deployment configuration opens as a separate modal task instead of stacking multiple information categories.

### 3. Catalog workspace

**Health: passed**

Strengths:

- Search, Provider filter, Search action, Browse action, and results now occupy one Data Panel.
- The previous “filter card + results card” visual split is removed.
- Table hierarchy is consistent with Providers, Profiles, Servers, Credentials, Audit, and Policy.
- Existing Tool detail/invoke Dialog behavior is preserved.

The Catalog Provider filter intentionally remains a native select because the current Base UI Select is difficult to exercise reliably in JSDOM and the project already uses native selects in several management forms. This preserves regression-test coverage without changing the visual IA.

### 4. Mobile Skills workspace

**Health: passed after repair**

The audit initially found a P1 responsive defect: the expanded 236px desktop Sidebar remained fully expanded at 390px, leaving the main content unusably narrow.

Repair:

- below `md`, Sidebar is now a fixed 68px icon rail;
- desktop still supports explicit expand/collapse;
- navigation links retain explicit accessible names when labels are visually hidden;
- language switching remains available on the mobile rail;
- PageHeader switches to vertical title/action layout;
- filters, list, and pagination regain usable content width.

The repaired mobile page is approximately 1731px tall versus roughly 2782px before the fix, reflecting the removal of the severe horizontal squeeze and resulting excessive wrapping.

## Shared visual system

Direction 2 is reflected in:

- cool white workspace background;
- quieter blue-violet primary accent;
- lighter border hierarchy;
- smaller 236px expanded Sidebar;
- 64px global top bar;
- compact page search;
- subtle active navigation state;
- reduced Card radius/shadow;
- unified Dialog overlay, spacing, title hierarchy, and footer;
- shared `DataPanel`, `WorkbenchToolbar`, `SearchField`, and `FilterSelect` for domain pages;
- `MetricStrip` and `MetricTile` are reserved for Overview only.

## Page-specific decisions

### Overview

Uses four real summary metrics:

- Runtime;
- Providers;
- Skills;
- Profiles.

Recent invocation activity is presented as one Data Panel.

### Providers

Provider statistics were removed from the domain page. The page now enters directly into the Provider inventory after title/actions and error state. Provider lifecycle actions and existing dialogs are unchanged.

### Skills

Skill statistics were removed from the domain page. The page now enters directly into the Skill Library with tabs, search, source/status filters, sort, list, pagination, and focused Dialog viewing.

### Policy

Policy statistics were removed from the domain page. The real configuration action for the default decision remains available as a compact toolbar inside the Policy Rules workspace; enabled-rule counts and approval-behavior summary cards were removed.

### Profiles / Servers / Credentials / Audit

These task-oriented inventories remain centered on one primary data workspace plus focused dialogs and do not expose decorative statistics.

## Audit findings and repairs

1. **P1 — mobile Sidebar consumed most of the viewport**
   - Fixed with a 68px icon rail below `md`.
   - Navigation accessible names retained.

2. **P2 — page IA varied by domain**
   - Fixed by introducing shared Command Workbench primitives and migrating all nine management pages.

3. **P2 — Catalog split search and results into unrelated Card surfaces**
   - Fixed by merging them into one workbench Data Panel.

4. **P2 — card-heavy hierarchy**
   - Reduced by using Cards only for meaningful grouped surfaces.
   - Domain-level statistic cards were later removed entirely; statistics are now reserved for Overview.

5. **P2 — shared Dialog styling lagged behind the new visual system**
   - Updated overlay, border, radius, spacing, heading hierarchy, and footer.

6. **P2 — prior design QA document was stale**
   - Updated during this commit audit to match Direction 2 and current evidence.

7. **Regression-test mismatch after shadcn Select adoption**
   - Skills Dialog regression test was kept focused on the Dialog contract.
   - Catalog uses a native Provider select so existing behavior tests remain deterministic.
   - Live-browser QA verifies the shadcn Skill filters.

## Browser diagnostics

Live browser QA has been run on representative pages:

- Skills;
- Overview;
- Catalog;
- Policy.

For the final representative QA runs:

- network diagnostics passed;
- console diagnostics passed;
- page error count: 0;
- screenshots were verified on disk.

Desktop Sidebar collapse and expand were manually verified after the redesign.

## Engineering verification

Current final gate:

- ESLint: PASS
- Workspace TypeScript: PASS
- i18n: **634 required / 634 zh-CN**
- Vitest: **9 files / 59 tests**
- Web production build: PASS
- Desktop production build: PASS
- `git diff --check`: PASS

The existing Vite warning for JavaScript chunks larger than 500 kB remains non-blocking and predates this design pass.

## Evidence limits

Screenshot/browser inspection can verify visible hierarchy, responsive behavior, interactive state changes, accessible names exposed by the browser, console errors, and network failures. It does not establish full WCAG compliance, screen-reader behavior across platforms, or all keyboard/focus-order behavior.

## Final result

**passed**

No unresolved P0, P1, or actionable P2 findings remain for this UI/UX commit.
