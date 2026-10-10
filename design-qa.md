# Mender Warm Canvas — design QA

**Date:** 2026-10-10  
**Visual target:** the Elera screenshots provided by the user (visual grammar, not healthcare data)
**Whole-app fidelity result:** **blocked** (remaining routes, themes and states not yet compared end-to-end)
**Focused management stage result:** **passed for inspected flows**.

## Design and workflow changes verified
- Shared `FocusSwitch` presents a single topic at a time using accessible, keyboard-focusable buttons with `aria-pressed`.
- Overview offers an activity view or resource preview view; at desktop width 1440 CSS px the four metrics render on one horizontal strip with computed columns of 279 px each.
- Plugin detail now shows **only** package overview, capabilities or technical metadata at a time. Add, enable/disable and removal remain explicit actions, without an inline configuration form.
- Runtime now separates status, budget preview and isolation preview. Editing continues in the pre-existing focused dialog flows.
- Sandbox creation is a staged route: choose backend → configure identity/authority → resource limits (Docker/Daytona only) → read-only review and explicit provisioning. The backend and service authority remain unchanged.
- Sandbox detail separates identity from security notes. Lifecycle actions are available only through a named Manage dialog; active leases disable actions, unsupported backend operations do not appear, and destruction requires confirmation.
- Shared action header wraps rather than overflowing at mobile widths. The metric strip uses Tailwind's standard `xl` breakpoint, fixing the prior 2×2 desktop grid.

## Rendered inspection — current isolated QA runtime
The earlier Pi Browser integration is still incompatible with its legacy journal. Rather than claim browser verification from builds alone, we launched a fresh, local **Chromium DevTools Protocol** session against a newly built Mender Runtime with **an isolated temporary SQLite database** (127.0.0.1:18081) and a temporary Vite frontend proxy (127.0.0.1:5176). A temporary local QA workspace was created; **no real user sandbox, credential or remote paid provider** was used.

| View / action | Evidence saved on QA host | Observation |
| --- | --- | --- |
| Overview at 1440×900, Chinese locale | `/tmp/mender-cdp-overview-1440.png` | 4 inline metrics; one active topic; width = documentWidth = 1440 |
| Marketplace → MCP plugin details → Technical details | `/tmp/mender-cdp-plugin-overview.png`, `/tmp/mender-cdp-plugin-technical.png` | Switch changed content; no stacked panels |
| Runtime status → budget preview | `/tmp/mender-cdp-live-runtime-status.png`, `/tmp/mender-cdp-live-runtime-budget.png` | One category per view; editing inputs absent in preview |
| Sandbox chooser at mobile 390×844 | `/tmp/mender-cdp-create-390.png` | 390 CSS px viewport, documentWidth = bodyWidth = 390, card width = 290 |
| Sandbox local identity configuration | `/tmp/mender-cdp-config-390.png` | Focused form after choosing a backend, not on inventory |
| Completed local review → Create sandbox → Detail | `/tmp/mender-cdp-local-review.png`, `/tmp/mender-cdp-local-detail-after-header.png` | Typed create succeeded against the isolated QA Runtime; navigated to the persisted sandbox |
| Detail → Manage dialog → Security boundary | `/tmp/mender-cdp-local-manage.png`, `/tmp/mender-cdp-local-security.png` | Actions gated inside dialog; security is a separate read-only topic |

Mobile after action-header fix: `documentWidth=390`, `mainWidth=322`, `cardWidth=290`. The three detail action buttons wrapped to two rows, with recorded horizontal boundaries between x=143 and x=374; none crossed the 390 px viewport. Chromium reported **zero JavaScript runtime exceptions** in the exercised workflow.

## Automation gates
- React/DOM tests: **91 passed / 15 files**. Covered plugin topic exclusivity, runtime focus and focused editing, Docker and Daytona step transitions and resource limits, local Sandbox lifecycle lease/confirmation gates, and navigation.
- `pnpm i18n:check`: **862 required / 862 zh-CN translations**.
- `pnpm typecheck`, `pnpm lint`, `pnpm build`: passed on the focused stage, including Web and desktop frontend builds.
- `git diff --check`: passed.
- `pnpm check`: **passed (exit code 0)** after removing the temporary QA-only Vite config. Includes Go lint/test/race, provider-plugin SDK and protocol checks, SDK code generation verification, desktop binding/architecture gates, frontend lint/typecheck/test/build. Log: `/tmp/mender-focused-fullcheck.log`.

## Limitations / remaining acceptance
- **Whole-app reference fidelity is not signed off.** Providers chooser, Profiles editing, Skills, Catalog, Credentials, Policy, Audit, Tunnels and other extended states still need equivalent side-by-side, same-state screenshot comparisons and interaction checks; the current screenshots do not justify claiming they are pixel-faithful.
- Only Chinese light-theme desktop/mobile screenshot samples were inspected this stage; dark mode, reduced-motion visual states, keyboard focus order and screen-reader behavior remain open.
- The isolated QA host had no verified Docker Engine or Daytona activation, so their create APIs were not exercised; only typed requests, limits and UI steps were verified in unit/integration tests. No remote provisioning was attempted.
- No healthcare-specific fictional metrics, Elera imagery or proprietary fonts were introduced into Mender.
