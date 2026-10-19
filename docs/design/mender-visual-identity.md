# Mender Visual Identity — Product Specification v1

> Based on the two sheets supplied in *Mender 品牌规范草稿.pdf* (Color / Type, Mendy). This production guide preserves the identity's source decisions and adds application rules. Additions are labeled “Product extension”.

## Position & voice

**Mender aggregates, governs and supplies the tools and resources agents run on.** It is an approachable, dependable tool-and-resource control plane, not an agent thinking engine. Source line: **Every tool, in its place.** Product copy should say what is available, what can be done, and what occurred without decoration or invented certainty. Do not reuse that brand line as a redundant subtitle on every page.

## Palette

| Source name | Value | Product use |
| --- | --- | --- |
| Care Green | `#7CD56E` | Selection, positive background and mascot body |
| Warm Canvas | `#F6F5F3` | Navigation and app canvas |
| Ink | `#121212` | Type, mark's dot and strong actions |
| White | `#FFFFFF` | Card, popover and dialog surfaces |
| Teal | `#02A794` | Secondary data accents |
| Soft Gray | `#F2F4F7` | Subtle control/background surfaces |
| Muted | `#5C5E63` | Secondary captions and supporting type |
| Sunlight | `#FFC954` | One meaningful new event; Mendy's new-tool port |

Source priority: green builds order, Ink represents the tool. Sunlight indicates one moment worth noticing, not broad surfaces.

**Product extension — semantic/accessible colors:** `#24642E` primary-ink small text; `#28874B` keyboard focus; `#E7E6E3` subtle warm border; independent semantic destructive red. Use semantic CSS variables throughout, including for dark-mode neutrals. WCAG contrast checks: Green / White **1.81:1**, Ink / Green **10.35:1**, Primary Ink / White **7.15:1**, Muted / White **6.49:1**, Teal / White **3.02:1**, Sunlight / White **1.53:1**. Never use green, teal or sunlight as small foreground text on white. Ensure status is conveyed through text/icons as well as color.

**Product extension — dark mode:** the source only defines the light palette. Preserve dark neutral surfaces and legible light foreground in dark mode, keep green with Ink foreground for primary selection, and do not invert Mendy artwork.

## Typography

- **Newsreader Medium**: display/headline at 24px and above, Mender wordmark (in public assets the wordmark is locked artwork). Never use for UI settings, table data, buttons or small labels.
- **Manrope**: body, interface, captions, data, controls; Regular/Medium/Semibold as needed. CJK fallback PingFang SC / Microsoft YaHei / system sans. Target body 16px, dense interface 14px, minor high-contrast metadata 12–13px (product exception); do not use sub-12px non-decorative information.
- Product scale (extension): display 32–36 / 1.15, page 29–32 / 1.15, panel heading 16–18 / 1.35, body 14–16 / 1.55, metadata 12–13 / 1.4. Tabular numerals for counts and timestamps. Avoid letter-spacing Chinese text.

## Identity assets

**Mender mark:** rounded open Care Green tool-case outline, one Ink dot in the opening. **Wordmark:** Newsreader Medium with open tracking. App lockup uses both; collapsed navigation and favicon use the mark alone. Keep clear space at least half the mark's height. Never stretch, overpaint, add a gradient/glow or recolor the dot. The exact source artwork takes precedence over an approximate re-drawing.

## Mendy: character with a job

Mendy is the portable case: green rounded body, handle, two Ink legs and **one White port/eye**. No mouth, nose or brows; the one port is the entire expression. Sunlight is restricted to the eye for a newly arrived tool.

Preserve source poses: **P0 Stand, P1 Lean In, P2 At Ease, P3 Turned, P4 Hop, P5 Carry**. Expressions: **00 Empty, 01 In Place, 02 Ready, 03 Focus, 04 Choose, 05 Done, 06 Idle, 07 New, 08 Miss**. Match expression to the actual application state. Do not show “Done” when a background job is unknown; “Miss” means unavailable/not found, not a technical failure.

Animation is strictly rigid transform: **rotate, mirror, scale and translate only**. Do not stretch the body or redraw the eye. Honor reduced motion. Use at most one mascot in a primary viewport, principally onboarding and meaningful empty/new/success states. Mascot never substitutes for instructions, status messages or accessible labels.

## Application design language — product extension

- **Canvas:** warm, calm frame around rounded white surfaces; subtle separators, reserved shadow. Surface radii 20–22px, buttons/fields 10–12px; 8px spacing scale; page gutters 24–32px, panel padding 16–24px.
- **Navigation:** open with mark + wordmark, collapse to mark; grouped sections; Care Green active item with Ink foreground; muted inactive links with visible hover/focus, min 40px desktop target. No new subtitle underneath every title.
- **Information hierarchy:** *one viewport, one information class*. Favor compact KPI strip + one chosen topic, not several unrelated dashboard panels. Preview before mutation; edits belong behind a clearly invoked modal or step.
- **Controls:** strong default actions Ink + White; green for intentionally emphasized actions with Ink; red remains destructive. Keyboard focus visible, hover/disabled clear. Long IDs truncated only with accessible full value.
- **Components:** cards, data panels, tables, badges, forms, dialogs and empty states share the same semantic CSS tokens. Primary actions stand out without visual noise. Never fill large blocks with Sunlight; reserve it for a single meaningful event.
- **Responsive / accessibility:** 44px touch targets, high-contrast text, accessible icon names, reduced-motion behavior, horizontal-scroll data tables; check light and dark UI.

## Implementation & acceptance

Shared foundations live in `packages/ui`, shell and pages in `packages/app`; both Vite web and Wails desktop consume them. No changes to agent/provider/sandbox execution or permission behavior. Implement assets and typography first, then shared tokens, shell and representative pages, then automated checks and rendered visual inspection.

Acceptance: brand silhouette, source colors and font pairing are recognizable; both themes accessible; collapse/search/language/status controls work; 1440×900 keeps one topic in focus; responsive pages do not clip actions; typecheck/lint/i18n/tests/build pass. This guide is the **production expansion** of the supplied two-page draft, not a claim that the PDF contains these additional rules.
