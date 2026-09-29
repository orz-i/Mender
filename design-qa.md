# Product Design QA — Mender Shell / Skills redesign

## Source visual truth

- Selected visual direction: Product Design option 3 ("Operator Console").
- Conversation source file: `file_00000000a61c81f5bffc8da1197e4b97`.
- Normalized source capture: `/mnt/data/product-design/mender-design-option-3.png`.
- Source pixels: 1536 × 1024.
- Visual source remains authoritative for Shell, Sidebar, content density, Skills list, filters, typography, and color hierarchy.
- Skill detail interaction is intentionally superseded by the user's later requirement: use the existing Mender detail Dialog pattern and keep one viewport focused on one information category.

## Implementation evidence

- Route: `/skills`.
- QA URL: `http://127.0.0.1:5174/skills`.
- Final list screenshot: `/tmp/mender-skills-dialog-list-final.png`.
- Final Skill overview Dialog screenshot: `/tmp/mender-skill-dialog-overview-final.png`.
- Resource-view evidence: `/tmp/mender-skill-dialog-resources.png`.
- Deployment-view evidence: `/tmp/mender-skill-dialog-deployments.png`.
- Browser viewport for final list/overview evidence: 1536 × 1024 CSS px, DPR 1.
- Locale: zh-CN.
- Data state: isolated temporary Mender Runtime with built-in `mender-tools`, local `code-review`, local `devops-assistant`, and exact deployments.

## Full-view comparison

The implementation preserves the selected Operator Console direction:

- grouped, collapsible left navigation;
- top command/navigation bar with Runtime connection state;
- module icon + page title;
- Skills tabs followed by search/source/status/sort controls;
- full-width Skill list as the primary page surface;
- restrained blue selection/action accents, pale neutral surfaces, light borders, and compact status badges;
- list/table-first information density rather than nested dashboard cards.

Intentional user-directed deviation from the concept:

- the concept's persistent right inspector is not used for Skill viewing;
- Skill details now open in the same Dialog interaction family already used by Catalog and other Mender management pages.

This is an explicit product requirement, not a fidelity defect.

## Focused-region QA

### Shell / Sidebar / Top Bar

Passed.

- Sidebar is grouped into Workspace / Runtime / Governance.
- Expanded and collapsed states preserve route structure.
- Collapsed state is icon-only with accessible labels/tooltips.
- Collapse state persists through reload/navigation.
- Language control remains accessible in both states.
- Quick Navigation opens from the top bar and supports page jump.
- Runtime connectivity comes from `getSystemStatus()`.

### Skills list

Passed.

- Built-in Skills are ordered before local installations.
- Search, source filter, status filter, and sort are real client-side controls.
- Rows use medium information density and independent action icons.
- "查看" is a dedicated Eye action and clicking the row also opens the same detail Dialog.
- The main page no longer carries an active Skill inspector, so list filtering and browsing do not leave stale detail context on screen.

### Skill detail Dialog

Passed.

The Dialog follows the existing Mender Catalog detail pattern:

#### Default: Skill details

Contains only installation identity and metadata:

- name / description;
- built-in/local status;
- enabled/disabled status;
- Installation ID;
- Skill ID;
- version;
- source;
- digest;
- installed state/time;
- resource count and total size;
- allowed-tools metadata when present.

It does **not** render the resource list or deployment list in this viewport.

The overview provides two explicit navigation actions:

- `View resources`
- `View deployments`

#### Skill resources

Contains only the validated resource inventory:

- path;
- resource kind;
- size;
- executable marker.

The resource list has an internal bounded scroll region and does not render deployment content.

#### Skill deployments

Contains only deployment information:

- target;
- drift state;
- destination;
- deployment time;
- undeploy action;
- optional Deploy Skill action.

The deployment view does not render resource content.

Subviews use a single `Back` action to return to Skill details.

The Dialog is bounded to `max-height: 90vh` with internal scrolling, matching the existing Catalog detail behavior on smaller viewports.

### Dialog stacking

Passed.

Starting a new deployment from the deployment detail view first closes the Skill detail Dialog, then opens the Deploy Skill Dialog. Two modal information categories are not stacked in one viewport.

## Interaction verification

Verified in browser with real isolated Runtime data:

- Skill list loads without a persistent inspector;
- View button opens Skill details Dialog;
- Skill details → View resources;
- Resources → Back;
- Skill details → View deployments;
- deployment drift/destination are visible only in deployment view;
- undeploy action remains available in deployment view;
- source/status/sort filters remain functional;
- sidebar collapse/expand and persisted state remain functional;
- Quick Navigation remains functional.

Automated test contract now explicitly asserts:

- overview does not contain `SKILL.md` or deployment destination;
- resource view contains `SKILL.md` but not deployment destination;
- deployment view contains deployment destination but not `SKILL.md`.

This makes the "one viewport, one information category" rule regression-tested.

## Browser diagnostics

Final attached Dialog QA:

- expected `技能详情` visible;
- console diagnostics passed;
- page error count: 0;
- network diagnostics passed with one benign preserved-session network warning;
- screenshot verified on disk.

## Intentional differences from the original visual concept

- Skill viewing uses Dialog rather than a persistent right inspector, per explicit user correction.
- Web does not expose arbitrary host filesystem Install/Import actions; those remain Desktop/CLI-only.
- The concept's Updates tab remains omitted because Mender has no Skill update lifecycle.
- Decorative tags/brand assets are not fabricated when no backing metadata exists.
- Browser/macOS chrome in the visual concept is not reproduced as app UI.

## Engineering verification

- ESLint: passed.
- Workspace TypeScript: passed.
- i18n coverage: 636 required / 636 zh-CN translations.
- Vitest: 9 files / 59 tests passed.
- Web production build: passed.
- Desktop production build: passed.
- `git diff --check`: passed.
- Existing Vite chunk-size advisory remains non-blocking.

## Comparison / repair history

1. P2 — shell proportions aligned to the selected Operator Console visual direction.
2. P2 — shared page identity strengthened with module icon tiles.
3. P2 — Skill list density and built-in ordering corrected.
4. P2 — functional source/status/sort controls added instead of decorative filters.
5. P2 — filter/inspector mismatch fixed during the first inspector implementation.
6. User-directed interaction revision — persistent Skill inspector removed and replaced with product-native Dialog viewing.
7. P2 — Dialog content split into mutually exclusive Details / Resources / Deployments views.
8. P2 — Deploy-from-detail now closes the detail Dialog before opening deployment configuration.
9. Dialog viewport bound aligned with Catalog: `max-h-[90vh]` + internal scrolling.
10. i18n and React 19 lint regressions introduced during redesign were resolved without weakening checks.

No actionable P0/P1/P2 findings remain.

## Final result

passed
