# Mender Dialog / Form Viewport Audit — 2026-10-12

Status: Implementation contract, verified against the user-supplied Profile screenshot and source inventory.

## Intent

When Profile Tools accumulate, **Manage Profile** currently renders every Tool binding in a single unconstrained dialog. The dialog grows beyond the viewport and its title/footer become inaccessible. Audit all dialogs and multi-field forms in the shared React app, keeping one category of information per viewport and preserving existing Profile permissions, ToolBinding and Plugin behavior.

### Evidence and inventory

- The supplied screenshot shows many tool rows (AnySearch, Workspace, Agent Delegation), icon-only row actions, the upper dialog outside the viewport, and its footer below the visible window. This is a **layout/interaction defect**, not a data or backend pagination defect.
- `packages/app/src/domains/profiles/profiles-page.tsx`: `Manage Profile` uses `sm:max-w-3xl` only, directly `map`s **all** bindings and intermixes Profile/Tunnel settings with the growing Tool list. The separate Tool Picker uses `max-h-[90vh] overflow-y-auto` at the root, causing the footer to scroll away; its three-column search form can overflow narrow viewports.
- `packages/ui/src/components/dialog.tsx`: shared Dialog popup has no default maximum viewport height, scrolling or overscroll containment. Any newly added long dialog can repeat the Profile defect.
- Inventory based on source search: **33 DialogContent instances** across **15** shared-app components (Profile, Provider, Skills, Policy, Credential, Tunnel, Sandbox, Server, Runtime, Catalog, Audit, Agent, Plugins, global shell). Dialog interactions are in shared `packages/app`, used by both Web and Wails Desktop; fixing only the desktop would create divergence.
- Existing bounded dialogs: Provider create, Credential create, Profile create, Sandbox access already have three-row constrained layouts and scrolling body. Catalog/Policy/Skills/Audit details have root scroll with explicit height; make sure fallback rules do not break them.
- High-risk unbounded multi-field dialogs: Provider authentication (`providers-page.tsx`: repeatable OpenAPI bindings), Tunnel create (`tunnels-page.tsx`) and Server start (`servers-page.tsx`), plus Profile per-Tool editors and large credential grants. Other short confirmation dialogs can rely on a conservative global viewport fallback.
- Long pages/forms: new Agent wizard already scopes creation to its own route and review; Provider create and Credentials create already have body scroll; avoid moving ordinary CRUD forms onto overview pages.
- **Limits:** the provided image proves the Profile overflow. Other Dialog assessments are source-based risk checks until browser QA captures their actual rendered behavior; do not claim screenshots or complete accessibility certification for unobserved flows.

## Design contract

1. **Dialog safety by default:** all dialogs stay within `100dvh - 2rem` and can reach every action via keyboard or pointer at laptop and compact widths. Scroll/overscroll should not trap focus or scroll behind the modal. Keep the existing Base UI close behavior; do not introduce a second overlay or route.
2. **Fixed chrome for complex modals:** for a multi-control dialog, use header, `minmax(0,1fr)` body, and footer; scroll only the content body. The Close/Cancel/Save actions remain visible regardless of row count. Root fallback scroll protects simple/legacy modals.
3. **One information category per view:** Manage Profile has **Overview** (identity, tunnel) and **Tools** (membership, search/filter, per-tool actions) tabs rather than simultaneous Settings and dozens of Tools.
4. **Scale by bounded views:** Profile selected Tools are searchable by title/name/alias/provider/description, filterable by Direct/Dynamic, and paginated using existing `usePagination` / `TablePagination` (default 10). Current Tool bindings are preserved even if not in the first 100 Catalog results. All four per-tool actions remain accessible and are not rendered for off-page rows. Search/filter reset the page.
5. **Tool Picker:** preserve selected set across pages and search, keep selection semantics/whole-set save, and make search controls responsive; a bounded list (and optionally page view) never sets popup height. No manual ToolID entry or nested modals.
6. **Other forms:** Provider authentication, Server start, Tunnel create and other multi-field forms use the same anchored footer and scrollable body where necessary. A shared simple Dialog max-height guard covers shorter confirmations, panels and already independently scrollable forms without a redesign.
7. **No behavior regression:** preserve Profile ToolBinding data, serial/policy/credentials editing, returns from per-tool editors to Manage, Profile Tunnel update, Wails/Web parity and i18n. Do not touch `.agents/`, `.pi/`, `.intent-loop/` or their contents.

## Verification and staged commits

- **Documentation first:** this audit/contract in a docs-only commit.
- **A — shared Dialog constraint and targeted Profile manager:** ensure overlay bounded; Manage Overview/Tools split, search/filter/pagination and pinned footer. Unit/UI tests with 60+ bindings and small-viewport class contract.
- **B — high-risk picker/forms:** Tool Picker and Provider authentication/Tunnel/Server forms gain header/body/footer scroll discipline, including narrow layout. Review remaining dialogs against the global fallback. Tests for selection preservation and form actions.
- **C — repository and visual acceptance:** run `pnpm check` and targeted UI tests; where possible, capture live Web/Desktop viewport and confirm top and bottom actions. Record any browser/device QA not performed instead of marking passed. Commit work in bounded functional increments.

**Non-goals:** modify APIs, impose server-side pagination on Profile Tool bindings, rename ToolIDs, add a new dialog framework, alter Mender navigation, or install a background UI agent.

## Implemented changes and audit disposition

### Profile management — verified regression

- `2482999`: the Profile `Manage` surface now has distinct `Tools` and `Overview` views. `Tools` is the default when the operator opens Manage; Tunnel and Profile identity are available under `Overview`, rather than consuming space above a potentially long list.
- The selected Tool list renders one page at a time (10 by default, with the existing 10/25/50 selector), searchable by Catalog title/name/description, Profile alias/overrides and provider title/namespace, and filterable by Direct/Dynamic. Empty, filtered, and paged states are separate. Per-Tool actions remain reachable, with truncated row text and icon tooltips. No ToolID input was reintroduced.
- `ToolPickerDialog` now uses a fixed viewport-bounded header/body/footer layout. The search and provider filter wrap to a vertical layout at small widths. Only the middle list scrolls; selection state is independent of pagination, query and provider filter. Saving still replaces the *complete selected membership*, not only visible rows.
- `profile-dialog-scale.test.tsx` exercises 65 selected Tool bindings, page navigation, filtering, and retaining selections across Tool Picker pages; existing Profile membership/governance, per-Tool presentation/credentials and Tunnel-binding tests remain green.

### Dialog/Form audit matrix

| Area | Source risk | Resolution | Evidence |
| --- | --- | --- | --- |
| Shared Dialog (all 33 instances / 15 components) | Popup could grow beyond viewport when content increases | Root `100dvh − 2rem` max height, overscroll containment, fallback root scrolling; reusable `DialogBody` for long forms | `dialog-viewport.test.tsx` |
| Profile Manager | Unbounded binding list; unrelated Tunnel configuration in same viewport | Overview / Tools tabs; filtered/paginated list; pinned footer | `profile-dialog-scale.test.tsx` (65 Tools), Profile regression tests |
| Profile Tool Picker | Full list and controls could make footer inaccessible, 3-column filter overflows narrow screens | Three-track constrained popup, responsive filters, 10-per-page inner list, independent cross-page selected Set | `profile-dialog-scale.test.tsx` and membership regression |
| Profile create / per-binding presentation, governance, credentials | Create was constrained; per-binding credential list can grow | Existing constrained create retained; default root viewport guard covers editors, credential list already scrolls | Profile membership / credentials UI tests + shared guard |
| Provider create | Existing complex form already had bounded body; risk of duplicate scroll | Preserved the established three-track create flow; root guard as fallback | Existing Provider flows |
| Provider authentication | Repeatable HTTP/stdio/OpenAPI credential rows without root bound | Fixed header/footer + scrolling `DialogBody` | `management-pages.test.tsx` Provider authentication assertion |
| Provider details | Long, variable plugin configuration metadata | Fixed header/footer + scrolling `DialogBody` | Provider detail assertion |
| Tunnel create / Server start | Multiple fields + descriptive notes | Fixed header/footer + scrolling body; no connection semantics changed | Tunnel and Server assertions |
| Policy rule editor / Runtime budget | Multi-step Rule or budget fields scroll footer offscreen | Fixed footer and scrollable step/field body | Policy and Runtime assertions |
| Catalog Tool detail / Skill detail | Long descriptions, schema blocks, resources/deployments | Fixed footer and scrolling middle content, no view-mode change | Catalog Tool and Skill UI tests |
| Credentials create, Sandbox access | Existing bounded three-track forms | Kept as-is with common viewport fallback | Source inspection; existing UI tests |
| Audit detail, Sandbox detail, Agent dialogs, Tunnels confirmation, Plugins and Shell | Typically short or previously internally scrolled; unconstrained future text risk | Common `DialogContent` guard prevents viewport expansion; no unnecessary form redesign | Source audit + shared guard test |

### Acceptance and limits

- **Automated gate:** `pnpm check` passed (exit 0). Go lint/tests/race, Plugin SDK/proto checks, Wails binding and architecture checks, TypeScript/ESLint, i18n (1055/1055), Vitest (28 files / 174 tests) and Web/Desktop frontend builds passed. Frontend builds still report a preexisting large-chunk warning, not a failure.
- **Browser/dev-state:** The existing Web and Wails Vite servers respond successfully. However, native Mac Pi browser access reported a legacy browser-journal validation failure; the environment blocked direct `agent-browser` CLI use. No after-fix real-window screenshot was captured, so visual pixel-fit, screen-reader behavior and physical keyboard navigation on a 320px × 480px viewport remain **unverified manually**. This is not claimed as a passed visual QA gate.
- **No backend or persistence changes:** Profile Tool bindings, ProfileIDs, ToolIDs, Credential grants, Provider and Tunnel configuration, and existing stored data are untouched. All shared changes affect both Web and Wails UIs. Dot-skill directories (`.agents`, `.pi`, `.intent-loop`) were excluded from commits.

### Remaining improvement candidates (outside this defect fix)

The Profile page initially hydrates Tool labels from at most 100 Catalog summaries. A Profile binding that is outside the hydrated Catalog page can still display `Unavailable Tool` instead of its title; its identity and authorization remain preserved. Consider a separate bounded/incremental metadata lookup or server-supported pagination if catalog size grows, rather than adding uncontrolled requests or changing the Profile schema in this UI fix.
