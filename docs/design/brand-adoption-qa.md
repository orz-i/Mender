# Mender brand adoption — first-pass QA (2026-10-19)

## Grounding
Source: supplied two-page brand draft; implementation rules in `docs/design/mender-visual-identity.md`. Browser inspected the running development app at 1440×900 and 390×844 using Chromium CDP. UI source shared by Web and Wails; Wails runtime itself was not independently verified.

## Acceptance checked

- **Desktop 1440×900:** accepted rendered screenshot `/tmp/mender-brand-visual-qa/01-desktop-overview.png`. Sidebar displays the one-port case mark and Newsreader wordmark; page title uses Newsreader; main canvas, compact metric strip, selected green nav and empty-state Mendy visibly render.
- **Typography:** `getComputedStyle(document.body).fontFamily` reports `Manrope Variable` first.
- **Mobile 390×844:** browser document width 390 matches viewport 390 (no page-level overflow at this route); screenshot `05-mobile-skills.png` from same URL (route remained Overview because the scripted English aria-label locators do not work in the Chinese locale).
- **Dark mode:** computed background `rgb(24,26,25)` and foreground `rgb(244,244,241)`; screenshot captured.
- **Errors:** no JavaScript runtime exception; current dev web service lacks API connection and shows `Mender API 请求失败。`. This is a service-availability blocker, not evidence of a brand rendering defect.
- **Automation:** `pnpm typecheck`, `pnpm lint`, `pnpm i18n:check`, `pnpm test` (47 test files / 286 tests), `pnpm build` passed.
- **Build warning:** Vite notes JS bundle >500KB (existing code splitting opportunity), build passes.

## Not yet established

- The full sidebar collapse / command palette interaction sequence is **not verified by this CDP script**, whose selectors assumed English labels while the local locale is Chinese. Existing unit tests still pass.
- Browser flows requiring a live Mender backend are blocked on the current isolated Vite preview.
- Pixel-level mascot fidelity cannot be certified because the draft is rasterized; current SVG is a controlled interface interpretation, not original master artwork. Replace with source master assets if they become available.

## Next visual pass

Review branded empty states across Skills / Agents / Sandboxes with live data or mocks, test keyboard/hover/focus and sidebar collapse with locale-aware selectors, compare Web and real Wails desktop, and consider splitting mascot source art into canonical assets. Keep brand work separate from operational lifecycle/permissions code.

**Verdict:** shared brand adoption passes automated compile and screenshot smoke check; production-wide visual QA **partially verified** pending backend-backed page journeys and desktop runtime.
