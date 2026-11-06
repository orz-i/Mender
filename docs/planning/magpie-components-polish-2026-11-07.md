# Mender · Magpie component refinement (2026-11-07)

## Goal and reference

The user wants a second-level fidelity pass across Mender's existing desktop/web screens, referencing the Magpie desktop UI screenshots they supplied: one horizontal global navigation, pale canvas, simple white surfaces, compact connected resource rows with recognizable icons, restrained inline status and minimal controls.

This is an **existing application**, not a prototype or a clone. Maintain Mender's distinct tool/resource governance behavior and all configured data. Do not invent model routing, usage balances or new backend contracts.

## Baseline (isolated Web browser, 1440×880)

- Shared `DataPanel` repeats decorative icon glyphs in every heading; 20+ pages inherit this, creating noise unlike Magpie's text-first sections.
- Empty `ResultPanel` often wraps an `EmptyPanelState` and a dashed `EmptyState`; compounded whitespace and borders obscure next actions.
- `DetailWorkspace` still allocates a fixed 172px left topic rail on desktop under an already horizontal global navigation; it compresses working content and diverges from the compact section navigation of Magpie.
- `Profiles` and `Sandboxes` have similar but independently styled resource rows; one uses a counter, the other colored badges and path, so spacing and actions vary.
- The common search field lacks an explicit one-click clear affordance and Escape-to-clear.
- Tools collection has a multi-button right edge that competes with the row's main resource action; needs hierarchy without hiding important management functions.
- Baseline routes `/`, `/tools`, `/profiles`, `/sandboxes`, `/runtime`, `/plugins`, `/skills`, `/audit` rendered without page-level overflow (fixture data). This is not native Wails verification; Mac mini's Wails window unavailable at audit time.

## Design system / acceptance

1. **Surfaces**: one outer card per section; border `var(--border)`, background `var(--card)`, radius ~14-16px; panel headings text-first at 13–14px, no decorative icon unless caller specifically requests one.
2. **Lists**: connected 76–80px rows with 36–40px icons, strong object name, one muted contextual line; quiet inline status (dot + text) instead of filled status pills, consistent keyboard focus and hover.
3. **Local navigation**: a single responsive, horizontally scrollable compact subnav above detail content (not another permanent left column). `aria-pressed`, section count and explicit labels remain accessible.
4. **Empty/loading/error**: one surface, clear sentence and concrete next action where the page has one; no nested dashed cards, no fake data.
5. **Search**: one search affordance with clear action and Escape, as well as keyboard labels; reset without losing pagination/query context.
6. **Responsive**: 1440×880 and 390×844; no body horizontal overflow; no clipped actions, accidental text overlap or hidden labels.
7. **QA**: preserve current page route/API/permissions and statuses; regression tests of shared primitives and representative Profile, Sandbox, Tool and detail navigation; lint, TypeScript, i18n, Vitest, web + desktop frontend build; browser screenshots and interaction proof.

## Staged work
- [ ] A. Refine shared panels, detail navigation, empty states, search.
- [ ] B. Apply consistent resource row/status anatomy to frequently used collections.
- [ ] C. Check legacy management screens for overflow and compound toolbars, fix evidence-based problems.
- [ ] D. Browser visual QA, responsive/accessibility, full regression and staged local commits.

**Boundaries**: no backend, database, ACP, Agent task state, permission or user-data mutation. No blind global CSS override. Use only existing explicit UI data. Do not push without explicit user request.
