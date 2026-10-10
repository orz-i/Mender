# Product Design QA — Mender Warm Canvas refresh

**Date:** 2026-10-10  
**Scope:** shared Mender Web/Desktop management UI  
**Visual target:** six Elera dashboard and design-system screenshots supplied in the current user request  
**Final result: blocked**

The implementation passes automated engineering checks, but **the image-to-code visual acceptance gate is not complete**. The native Mac Pi browser tools return a legacy browser journal validation error before any navigation, screenshot, DOM inspection, console inspection or interaction. Do not reuse the earlier Command Workbench QA's passed verdict for this refresh.

The earlier unrelated QA is preserved at `docs/development/command-workbench-design-qa.md`.

## Source-to-implementation inventory

| Source design evidence | New Mender implementation | Visual inspection status |
| --- | --- | --- |
| Brand colors: green `#7CD56E`, canvas `#F6F5F3`, ink `#121212`, white, teal, muted, sunlight | Shared tokens in `packages/ui/src/styles/globals.css`; dark theme uses semantic equivalents | Tokens inspected; rendered colors **not captured** |
| Typography: strong sans headings and compact KPI labels | Inter-first with Chinese fallback; larger page title/numeric KPI, muted secondary text | Code inspected; font rendering **not captured** |
| Sidebar: grouped navigation, green selected row, soft background | Mender grouped nav with active state, language selection and collapse/expand | Unit tests passed; responsive visual **not captured** |
| Dashboard: continuous white metric strip, dark icon tiles, rounded data panels | `MetricStrip`, `MetricTile`, `DataPanel`, `Card`; Overview uses actual recent calls/provider/profile data | Live DOM/screenshot **not captured** |
| Administration: pill controls, readable tables, white panels | Shared button/input/select/badge/table restyle, Marketplace and Skills accents; all domains reuse shared primitives | Creation-flow unit tests passed; actual route screenshots **not captured** |

Healthcare-specific content from references was *not* transplanted to Mender. MeyaSans is reference typography only; no external font asset is added.

## Automated engineering checks

- `pnpm test`: **15 files, 87 tests passed**, including new quick-navigation and sidebar tests.
- `pnpm i18n:check`: **856 required / 856 zh-CN translations**.
- `pnpm typecheck`: passed all workspaces.
- `pnpm lint`: passed.
- `pnpm build`: Web and Wails desktop bundles built.
- `git diff --check`: passed.
- Vite emitted an existing nonblocking JavaScript chunk-size warning.

## Browser follow-up required to unblock design acceptance

1. Resolve the Mac Pi legacy browser-journal incompatibility, or use another authorized supported browser session.
2. Capture Overview, Marketplace/detail, Providers/add chooser, Skills/detail, Sandboxes/create, Profiles, Runtime, Policy, Credentials and Audit at desktop (~1440×900).
3. Capture Overview, Marketplace, and one representative table/form at mobile (~390×844).
4. Compare palette, navigation, card geometry, typography, dividers, controls, focus states and dark-mode contrast with the supplied source images. Fix all P0–P2 mismatches, then recapture.
5. Verify quick navigation (⌘/Ctrl+K), sidebar collapse, route changes, provider creation, locale toggle, filters, table overflow, empty/error states and primary dialogs.
6. Inspect console/page/network errors. Only after screenshots and interaction proof are reviewed, change `final result: blocked` to `final result: passed`.

## Outstanding risk

There are **no current live screenshots** for this redesign, so screenshot fidelity, absence of clipping/overlap at responsive sizes, and complete browser accessibility have not yet been verified. Code and unit/integration checks pass; visual sign-off remains pending.
