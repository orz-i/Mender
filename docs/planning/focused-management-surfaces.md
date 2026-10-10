# Focused management surfaces — follow-up stage

## Decision and scope
Continue the accepted Mender warm-canvas redesign with the user's stronger interaction rule: **one viewport focuses on one information class; preview is read-only until an explicit action initiates mutation**. This stage is UI-only, preserving runtime APIs, authority checks, routing and capability boundaries.

## Baseline audit
| Surface | Before | Target |
| --- | --- | --- |
| Plugin detail | Three stacked panels for package identity, capabilities and technical metadata; competing above-fold content | Compact topic switcher: Overview / Capabilities / Technical details; render exactly one topic panel at a time, retain lifecycle and Add actions |
| Sandbox detail | Eight identity fields plus a separate security panel and a high-density action bar | Overview / Security boundary focus switch; lifecycle actions entered via one explicit Manage control, with existing confirmations and lease gates |
| Sandbox create | Backend chooser followed by a potentially 8+ field configuration screen and review | Explicit four-stage flow for Docker/Daytona: Backend → Identity → Limits → Review; local workspace skips Limits. One configuration category visible per stage, review remains read-only and auditable |
| Runtime | Overview metrics + budget + isolation previews simultaneously | Status / Budget / Isolation focus switch with editing only in the existing focused dialogs |
| Providers, Profiles, Credentials, Policy | Inventory previews already lead to create/edit dialogs | Preserve those contracts; audit and test for regressions, do not collapse specialized dialogs into inline forms |
| Marketplace, inventory pages | Card/list previews already point to details | Preserve cursor/search/pagination; focus on action and viewport controls rather than adding more blocks |

## Shared focus-switch contract
Introduce a small, semantic, stateless `FocusSwitch` in `packages/app/src/shared`:
- accepts `value`, accessible `label`, literal `items` and `onChange` (no router / client / backend dependency)
- uses native buttons with `aria-pressed` for selection and a named group; no tab role without corresponding tab semantics
- respects existing focus-visible theme, touch targets, localization, and wraps rather than overflows
- no hidden form sections are mounted in the main content while the other topic is selected
- use the same control across pages; do not add another dependency.

## Behavior invariants
- All Sandboxes are real client resources; no placeholder data, simulated state, implicit provisioning, or fake running state.
- Sandbox limits stay enforced by existing UI validation and backend admission. No backend activation, bypass, or new compatibility path.
- On lifecycle action, respect `activeLeases` gating; destructive removal requires the existing explicit user confirmation. Manage dialog cannot be used to expose unsupported actions.
- Docker and Daytona create send exactly the same typed request fields, and cannot skip read-only Review.
- Errors remain visible and actionable, editing remains isolated from preview, and modal opens contain explicit titles.
- All existing English/Chinese translations continue to pass coverage checks; new labels must be translated.
- Keep `.agents/`, `.pi/`, `.intent-loop/` out of commits.

## Delivery slices
1. This document, committed first.
2. Shared FocusSwitch + Plugin detail and Runtime, tests and commit.
3. Sandbox create and detail progressive disclosure, regression tests and commit.
4. Build/lint/typecheck/i18n/tests, local desktop and narrow-screen screenshot plus interaction checks; update `design-qa.md` with evidence and remaining limitations. Only mark visual acceptance passed if genuinely inspected.

## Exit criteria
- One topic panel per Plugin/Sandbox/Runtime detail view; no three-card vertical information stack.
- No editing controls on ordinary inventory/preview content. Explicit View/Edit/Manage operations lead to focused screens/dialogs.
- Docker/Daytona configuration is split between identity/authority and resource limits. Local config remains concise.
- Regression coverage for switching topics, Back/Review, guarded actions, and correct requests; Web & Desktop builds green.
- Desktop (~1440×900) and narrow (~390×844) rendering captured and inspected with findings recorded.
