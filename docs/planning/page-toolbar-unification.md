# Phase: page-level toolbars across all Mender management pages

## User intent and visual reference
The user-provided Skills screenshot shows a repeated card header and internal rows for category tabs and search/filter controls; the Elera references instead place **Tabs + search/filters + primary actions in one page-level toolbar, outside the result card**, reserving the white rounded card for results/pagination. This change must apply across *every* Mender management page, not only Skills.

## Acceptance contract
- The management shell alone owns each page's top title. No duplicated visible "Skill Library", "Plugin catalog", "Provider inventory", etc. inside a result card.
- On inventory pages, **one contiguous, responsive page-level toolbar above and separate from the result card** contains available category tabs, search/filter/sort and Refresh/Add/Edit actions; left aligned groups then right aligned controls, wrap only when necessary. Do not split the controls between a top-right PageHeader and a second card toolbar.
- The result card contains only a list/grid, its empty/loading state, and pagination. Include a visually hidden accessible section title where needed. Category tabs are not artificial; only show categories supported by existing state/filters. Do not add fake categories/counts.
- Longer descriptions, IDs and safe diagnostic metadata remain on focused detail views, not duplicated as marketing text in result headers. Keep existing read-only previews, dialogs and confirmation paths.
- Dedicated detail or create routes may retain a focused information heading/description inside their own card and action controls outside. A wizard's Back/Next/Review/Submit still belong to the step they control; do not turn them into global list filters.
- Use one shared `PageToolbar` for all page toolbars and one shared `ResultPanel` for inventory results. Keep the existing `DataPanel` for scoped detail panels, not duplicate its heading for simple inventories.
- Follow reference palette/spacing, preserve Chinese & English labels, affordances, keyboard focus, responsive behavior, date/status semantics and table pagination.
- No backend, contract, API, sandbox activation, Principal/Policy, persistence, or tool execution changes. No dependencies.

## Route inventory and decisions

| Route / screen | Tabs / filters / actions | Result / detail treatment |
| --- | --- | --- |
| Skills | Existing All/Built-in/Installed/Enabled tabs + search/source/status/sort + Refresh/Install together | ResultPanel with rows + pagination |
| Plugin Marketplace | Search/type/source/status filters + existing installer actions; no invented category tabs | ResultPanel with card grid + cursor pagination |
| Providers | Existing Add action; add local search from safe inventory metadata | ResultPanel with table + pagination |
| Catalog | Existing intent search + Provider filter + Search/Browse actions | ResultPanel with tool table + pagination |
| Profiles | Existing Create action; add local search by profile name/slug | ResultPanel with table + pagination |
| Credentials | Existing Add action; add local inventory search by display name/namespace/source metadata (not secret value) | ResultPanel with table + pagination |
| Policy | Default decision read-only preview + Edit and Add rule in page toolbar | ResultPanel with rule table + pagination |
| Servers | Existing Refresh/Start actions together | ResultPanel with running server table + pagination |
| Tunnels | Existing Add action and optional on-demand information; no persistent large explainer above list | ResultPanel with tunnel table + pagination |
| Sandboxes | Existing backend/search filters + Create in one toolbar; category metrics are separate optional overview, not embedded controls | ResultPanel with inventory table + pagination |
| Audit | Existing Invocation/Provider history FocusSwitch + search/status filter + Refresh in same PageToolbar | ResultPanel with table + pagination |
| Sandbox activity | Existing event-type filter + Back action in page toolbar | ResultPanel with event table + pagination |
| Overview | Existing activity/resource FocusSwitch already outside cards; use PageToolbar and keep focused metric/preview composition | Existing cards are actual dashboard content, no fake list card |
| Runtime | Existing Runtime/budget/isolation FocusSwitch outside focused content; use PageToolbar | Actual metrics/detail preview stay in their separate focused card |
| Plugin details, Sandbox detail, creation/management flows | Move route-level navigation/lifecycle actions into toolbar when practical; preserve their focused content, dialogs and wizard semantics | Do not erase necessary detail headings or configuration labels |
| Skills, Provider, Profile and related dialogs | Within-dialog controls remain inside their focused operation | No form shown on inventory by default |

## Implementation and QA slices
1. Document first, commit.
2. Shared primitives + Skills and Plugin Marketplace. Regression tests, commit.
3. Providers, Catalog, Profiles, Credentials. Regression tests, commit.
4. Policy, Tunnels, Servers, Sandboxes, Audit, Sandbox activity, Overview, Runtime and focused detail/creation route alignment. Regression tests, commit.
5. Real Chromium desktop (~1440×900) and narrow (~390×844) inspection for each route where possible; use synthetic data only in isolated QA and label it clearly. TypeScript, i18n, lint, unit tests, Web and Wails frontend production builds; final QA report and commit.

## Non-goals and risk
- The user specifically wants **layout hierarchy**, not new dashboard metrics or product features. Avoid adding arbitrary tabs, redundant counters or permanent new search state to routes when unnecessary.
- Existing actions must not silently change from create/edit dialogs to inline forms. Never silently mutate via a filter or tab change.
- Keep `.agents/`, `.pi/`, and `.intent-loop/` out of Git commits.
