# Audit workbench — focused preview and drill-down

## Problem / evidence
The supplied 2026-10-10 screenshot of Mender's Audit page shows two large stacked tables (Invocation history and Provider activation history). The first repeats seven metadata columns, raw English machine statuses (`completed`/`failed`/`allow`/`execution`) inside an otherwise Chinese UI, and a full-width pagination area. The second begins below the first in the same viewport, violating the established **one viewport / one information class** and **preview before edit/action** rules. The current detail modal simultaneously lists ~20 metadata fields, making failure diagnostics harder to find; some IDs use truncation without any reveal.

## Goal
Audit remains a **read-only, data-minimized history workbench** with an explicit switch between **tool invocations** and **provider activations**. One table (or one empty state) occupies the active viewport. Clicking the eye icon opens a focused **read-only detail dialog**, initially showing diagnostics for failures and overview for completed records. No form or mutating controls appear on this page.

## Layout
1. Header: route title owned by the global shell, compact Refresh action.
2. Section selector: reuse `FocusSwitch` with exactly two localized entries. Never stack both tables.
3. One `DataPanel` containing a compact search and status-filter toolbar, one table with clear preview columns (`Finished`, target name/ID, `Principal`, localized semantic `Status`, `Policy`, view action), and the existing cursor-free page-size/page-number controls.
4. Keep one horizontal scroll region for narrow viewports rather than shrinking text or clipping action controls. Empty and zero-result states are distinct.
5. Render status names as readable localized labels and semantic badge colors; preserve raw machine codes in a field only where they are diagnostic facts. Do not treat unknown statuses as success.
6. One dialog at a time: `Overview` / `Governance` / `Diagnostics`. Completed records open on Overview, failed/denied records open on Diagnostics. Let users switch topics; long IDs and safe summaries wrap and scroll, not truncate.

## Behavioral and safety boundaries
- Preserve both existing `listAudit(100)` and `listProviderActivationAudit(100)` contracts and optional tool-title lookup; do not change API, storage, provenance or retention.
- Search/filter only locally against returned **safe metadata** (time, status, tool/provider ID, principal, error class, policy kind); never index or display Tool arguments, response bodies, raw secrets, credentials or provider configs.
- Do not falsely claim that the latest 100 records are the complete history. Counts are counts of retrieved/filtered rows only.
- Keep both independent pagination states; topic and filter changes should not leave users on stale/out-of-range pages.
- Explicit Refresh must remain available; UI needs loading, failures, no-records, no-match states without data fabrication.
- Preserve original `errorSummary` / `errorClass` and known failure codes inside the detail view, with fallback for missing diagnostic summary.
- Maintain Chinese/English i18n, focus states, keyboard access and mobile overflow handling.
- No compatibility bridge, dependency or backend changes. Do not add `.agents/`, `.pi/`, or `.intent-loop/` to commits.

## Implementation slices
1. This design plan committed alone.
2. Audit list workbench: FocusSwitch, search/status filtering, semantic badges, compact columns and single table. Verify.
3. Audit detail: focused overview/governance/diagnostics sections; regression tests for failures and provider activation data. Verify.
4. Real Chromium desktop/mobile screenshot QA against a test-only runtime; TypeScript, lint, i18n, unit tests and builds. Record evidence/known limits and commit.

## Acceptance
- On a 1440×900 viewport, the second history table is **not** rendered until its selector is activated.
- Machine status labels in list are translated and distinguish completed, failed, denied/other.
- Failure dialog displays the safe error summary when opened, without requiring scrolling through identity fields; governance/identity remain accessible on explicit switch.
- No editable form or dangerous action is embedded in Audit.
- 390×844 mobile view keeps selection/search/row action reachable and does not overflow the overall document horizontally.
- Existing management page tests pass; additional tests cover topic switching, search/status filter, pagination, detail section switching and missing summary.
