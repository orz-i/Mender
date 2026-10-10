# Audit workbench — focused UI QA

Date: 2026-10-10
Scope: `/audit` in the shared Mender Web/Desktop frontend
Overall Elera whole-application design sign-off: still tracked separately by `design-qa.md`
**Scoped Audit workbench QA result: passed** for the inspected browser layouts and interactions.

## Reference and intended difference

The user-provided Audit screenshot dated 2026-10-10 showed a full-width Invocation history table, untranslated `completed`, `failed` and `allow` values, and the second Provider activation history card occupying the same screen below it. The dialog exposed every identity/governance/diagnostic field together.

The intended improvement is progressive disclosure rather than an unrelated product dashboard: one table per view, localized semantic outcomes, compact search/filter/refresh, details reached explicitly from a row, and a focused failure-first diagnostic screen.

## Browser evidence

Mac Pi's native browser journal is incompatible with this legacy session. We verified the app in an isolated headless Chromium DevTools Protocol session through a temporary script outside the repository, against the live Mender Vite app on `http://127.0.0.1:5175/audit`.

The QA script intercepted **only the two Audit history read endpoints** with synthetic, bounded non-sensitive QA fixtures. The old dev Runtime's Provider activation endpoint currently returns 404, so these screenshot fixtures **are not claims about real audit data or current backend availability**. No fixture or debug route was added to the application; no audit records or secrets were written to the user database.

Screenshot files on the QA Mac (also exported to the conversation as image attachments):

- `/tmp/mender-audit-new-desktop.png` — 1440×900 Chinese invocation preview, five QA records
- `/tmp/mender-audit-new-activations.png` — provider activation topic switch
- `/tmp/mender-audit-new-diagnostics.png` — failed invocation opens directly on safe error summary
- `/tmp/mender-audit-new-governance.png` — governance topic, instead of a long multi-section modal
- `/tmp/mender-audit-new-mobile.png` — 390×844 narrow summary rows and visible View actions

### Observed checks

| Check | Result |
| --- | --- |
| Page identity and nonblank screen | `/audit` loaded; meaningful title and localized history table rendered |
| One active class of information | Exactly **1 table and 1 data panel** on invocation and activation screens |
| Status readability | Completed green, failed red, denied warm amber; policy Allow/Deny translated |
| Desktop compact layout | One table card with toolbar and pagination; no second stacked history card |
| Failed invocation detail | Bounded diagnostic summary visible immediately on opening row; governance revealed by switch |
| Interaction proof | Switched to Provider activation history, then returned to invocation; opened details and changed section |
| Mobile layout | `innerWidth = documentWidth = 390`; table `scrollWidth = clientWidth = 288`, no page/table horizontal overflow |
| Mobile row action | View button horizontal bounds x=329..357 within the 390 CSS-pixel viewport, no scrolling required |
| Framework overlay/console errors | No framework overlay or relevant JS runtime exceptions observed |
| Privacy | Only audit API safe summaries/metadata displayed; arguments, result bodies and credential material absent |

Mobile summary rows were tightened after the initial capture, replacing a horizontally scrollable desktop table. The refresh button is icon-only at narrow widths with an accessible label, and the detail dialog's Close action is translated. Longer identifiers remain readable on demand within details.

## Automated checks

- 16 Vitest suites and **97 tests** passed. Dedicated Audit tests cover topic exclusivity, local search/status filtering, pagination reset and empty filter state, failed-first diagnostics, Provider activation drill-down, refresh, and searching visible Chinese failure statuses.
- i18n coverage **872 required / 872 zh-CN translations** passed.
- TypeScript, ESLint, `git diff --check`, and Web plus Wails desktop frontend production builds passed.
- Existing Vite chunk-size warnings remain nonblocking.
- Full `pnpm check`: **passed (exit code 0)**, including provider-plugin SDK/protocol checks, Go lint/test/race, generated contract checks, and all frontend gates. Log: `/tmp/mender-audit-fullcheck.log`.

## Out of scope and limits

- No backend schema/retention changes, database migrations, mutation workflows or remote provider calls.
- Does not certify exact screenshot-pixel matching to Elera's healthcare application; the visual grammar and Audit layout were the comparison target.
- The CDP visual pass used synthetic Audit history to exercise all UI states. Live provider-activation-history integration requires an up-to-date Runtime and genuine records; not claimed here.
- Complete dark-mode contrast checks, every device viewport, screen reader behavior and all other Mender domains remain subject to separate product-wide QA.
