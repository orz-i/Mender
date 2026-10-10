# Mender UI visual refresh — warm canvas design system

## Problem and outcome
Mender's management application uses a technically consistent set of shadcn primitives but presently reads as a dense, cool-colored management console. The supplied Elera references establish a specific visual language: warm canvas, bright green selected navigation, ink typography, restrained dark icon tiles, white rounded information surfaces, lightweight control pills, and clear data hierarchy. Translate **the design language, not Elera's healthcare features or brand**, into the existing Mender tool-and-resource control plane.

## Scope and sequence
1. **Foundation**: Shared light/dark semantic tokens, consistent typography, radius, elevation, focus/hover states and color accessibility. Preserve the existing UI package and React 19/Tailwind 4 stack.
2. **Application shell**: Mender identity, grouped navigation, green active item, compact controls, route title, quick navigation and responsive/collapsible sidebar. All current routes and locale switching remain usable.
3. **Shared information system**: unified four-metric strip, data-panel header/toolbar, tables, pagination, cards, badges, inputs, selects, buttons, and empty/error states.
4. **Domain pass**: test Overview, marketplace/detail, Providers, Skills, Catalog, Sandboxes, Profiles, Runtime, Servers, Tunnels, Credentials, Policy and Audit, including create/detail dialogs.
5. **Validation**: automated checks, desktop/mobile visual inspection of the working application, keyboard/interaction smoke checks and QA notes. Separate meaningful commits by layer.

## Foundations (light)
| Role | Reference | Implementation role |
| --- | --- | --- |
| Care green | `#7CD56E` | selected navigation, emphasis and positive states |
| Warm canvas | `#F6F5F3` | app and sidebar backdrop |
| Ink | `#121212` | body, headings, icon tile surface |
| White | `#FFFFFF` | cards, panels, overlays |
| Teal | `#02A794` | secondary data accent |
| Soft gray | `#F2F4F7` | muted controls and neutral chips |
| Muted | `#5C5E63` | supporting text; check contrast for small labels |
| Sunlight | `#FFC954` | attention/warning surfaces, never a replacement for semantics |
| Derived | warm neutral borders | dividers and focus-visible treatments |

Typographic direction: **Inter first**, with local system/CJK fallbacks; MeyaSans is reference-only unless a properly licensed font is already available. Page title 24–28 px medium/semibold, panel title 15–17 px, body 13–14 px, muted supporting text 12–13 px. Metrics 24–28 px with tabular figures. Avoid overly light, low-contrast gray text.

Geometry: large white panels with ~20–24 px radius and restrained elevation, 12–14 px buttons/inputs, navigation target height >= 40 px, consistent ~16–24 px interior spacing, 24–32 px primary page gutters. Prefer a single continuous metric strip with dividers over four unrelated bordered boxes.

Controls and state:
- Care green backgrounds carry **ink text** for contrast. Primary action buttons may use ink surfaces with white labels to reserve green for selection and positive emphasis.
- Info/status colors remain semantic and are not inferred from green/orange decoration alone.
- Keyboard focus-visible rings, disabled, hover and selected states must remain distinguishable in both themes.
- Truncate long provider/tool names and descriptions, but keep details accessible in appropriate views and dialogs.
- Preserve table horizontal overflow at small widths; never clip actionable cell controls.
- Honor reduced-motion and avoid needless transitions.
- Keep Chinese and English labels and the existing navigation/search behavior intact.

## Dark theme
The reference primarily defines light mode. Dark mode retains the same hierarchy with a low-chroma ink canvas, elevated dark panels, legible warm white foreground and accessible care-green accents. Do not display light-only cream/white hardcoded surfaces inside dark mode.

## Implementation boundaries
- UI-only Class C change. No API, provider, sandbox, policy, persistence or backend behavior changes.
- No new framework or dependency and no duplicate per-page theme CSS.
- Reuse the established `@mender/ui` primitives and `packages/app/src/shared` components.
- Never include `.agents/`, `.pi/`, or `.intent-loop/` in commits.
- No copied healthcare metrics, patient content, Elera logo, or proprietary font files.


## Viewport information and action discipline (2026-10-10 clarification)
- **One viewport, one information class**: A screen's primary surface focuses on one topic (activity, resources, governance, or configuration). Do not stack an activity table, status breakdown and resource inventories in the same initial viewport. If adjacent topics are useful, expose a clear view switch or detail navigation instead of competing dashboard panels.
- **Preview before mutation**: Inventory and detail pages show read-only summaries first. Explicit Add/Edit/Delete actions open a dedicated focused dialog, wizard step, or route; do not render editable configuration fields directly in the normal inventory/preview viewport. A dedicated creation route is acceptable only if it starts with a choice/review step, followed by a focused configuration step, rather than showing every setting immediately.
- **Progressive disclosure**: Dense parameter forms must be segmented into logical steps with accessible Back/Review/Confirm controls; destructive actions need explicit confirmation. Existing Providers/Profile/Credential create flows already use dialogs; preserve these boundaries.
- **Overview specifically**: Show one compact four-item overview strip and a selectable **activity vs resource** area. Activity view presents recent invocations. Resource view presents Provider/Profile previews. No duplicated activity summary cards on the same viewport.
- **Acceptance**: At 1440x900 the Overview initial viewport shows only the KPI strip and the active topic panel; switching topics is explicit. No mutating fields appear in any list preview without an initiating action.

## Acceptance
- Existing routes and dialogs remain functional, including quick search and sidebar collapse.
- On desktop the shell, metric strip, panels and navigation align with the supplied palette, spacing and hierarchy.
- Narrow widths have no overlapping title/search/nav/actions or clipped primary controls; tables scroll where necessary.
- Both light/dark mode are usable; meaningful focus and sufficient foreground/background contrast.
- TypeScript, lint, i18n and UI tests pass.
- Real browser QA includes at least an Overview screen plus representative data and creation screens; no claim of visual verification without rendered screenshots.
