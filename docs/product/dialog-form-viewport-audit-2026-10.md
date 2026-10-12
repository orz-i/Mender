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
