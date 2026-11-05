# Agent preset directory — Magpie-style UX verification (2026-11-05)

## Design reference and scope

- User-provided current Agent toolbar screenshot: three status tabs, search input, "Add agent" button.
- User-provided [Magpie Agents screen](https://github.com/yetone/magpie/tree/main/site/public/img): a single thin-bordered, white, divided directory with branded marks, short human-facing descriptions and inline status.
- Mender is not Magpie. It governs configured Agent instances and Profile/Task grants; an Agent *backend preset* is different from one configured workspace. Preserve that distinction in status and navigation.
- Four built-in presets are explicitly registered: **Codex, Grok, OpenCode, Antigravity**. No fabricated future backends or hidden default Codex fallback.

## Implementation audit

1. Replaced old tabs/filter/search/Add Agent action with a single responsive list of four preset rows.
2. Each row has an offline brand mark (LobeHub Icons, MIT with notice at `packages/app/src/assets/agent-icons/`), preset name, concise description, status dot/text, conditional number of configured workspaces and keyboard-focusable destination.
3. Actual states from independent Host/plugin catalogue and Agent resource reads: *CLI available/unavailable for configured resources*, *ready to configure* for host-supported but unconfigured preset, *backend unavailable* if host disallows, and *status unverified* if data cannot be checked. An error is not displayed as an empty inventory.
4. Click routing uses exact backend/Agent IDs:
   - One configured workspace → `/agents/:agentId?view=configuration` (configuration section selected immediately).
   - Multiple configured workspaces → `/agents/backend/:backend` to choose exact workspace, then configuration.
   - No configured workspace and healthy host preset → `/agents/new?pluginId=...&from=agents` (pinned configuration, then review & save).
   - Unavailable/unknown preset → backend status page with explanation, not fictitious create action.
5. Removed the redundant *generic* backend chooser: unpinned `/agents/new` redirects to `/agents`. Marketplace `create.agent` still pins a known plugin ID; backend, data model, safety review, Task, grants and ACP implementation unchanged.
6. No real user data changed. No changes to backend or database schema. No permission bypass or Agent Loop takeover.

## Browser QA evidence (fixture data, not user data)

| View | Screenshot | Verified |
|---|---|---|
| Desktop directory, 1440×900 | [preset-directory-desktop.png](preset-directory-desktop.png) | All four presets, brand marks loaded, no search/tabs/add, inline status and exact destinations |
| Mobile directory, 390×844 | [preset-directory-mobile.png](preset-directory-mobile.png) | Four visible rows; `scrollWidth=390` with no horizontal overflow |
| Multiple Antigravity workspaces | [multiple-workspaces.png](multiple-workspaces.png) | Choose exact configured resource, not a random first match |
| Existing OpenCode configuration | [existing-agent-configuration.png](existing-agent-configuration.png) | Direct navigation to selected Configuration section; existing backend/CLI/isolation/workspace fields accessible |

The browser interactions were verified using isolated **GET API fixtures** from a temporary Web dev server on port 5190. No real Agent commands were executed and the fixture data is not authoritative Host availability.

Additional checks:
- Click Grok → pinned setup with no backend chooser; no `Add agent` button.
- Click Codex unavailable → diagnostics; no fake setup entry.
- Missing Agent inventory / plugin catalogue → unverified/retry rather than falsely empty or available.
- Chrome language and breadcrumb display correct semantic configuration label.
- Brand assets load, without network access to third-party logos; MIT license retained.
- Chromium `axe-core 4.12.1`, Agent directory 390px: **0 WCAG 2 A/AA / 2.1 AA violations; 0 incomplete, 21 passes**.

## Automated gates

- ESLint all apps/packages: pass.
- TypeScript all packages: pass.
- i18n: **1639/1639** required Chinese translations, obsolete Agent inventory UI messages cleaned.
- Vitest: **75 test files / 507 tests passing**, including preset selection, exact identity, unavailable status, all four presets, config and older Task/approval flows.
- Web and Wails frontend builds: pass. Existing >500kB main bundle warning remains unrelated.

## Wails native verification

During this turn, the Wails dev process remained running but the native OS accessibility window enumeration returned **0** and a native `screencapture` returned a black frame. A browser view of the dev URL lacked real Wails bindings. Therefore: **native post-change screenshot and real-data interaction acceptance are not verified**. Rebuild/reload the original native window and inspect the four preset rows, the exact configured Agent, CLI unavailable diagnostics, keyboard navigation, and permission controls before accepting full desktop QA.

**final result: blocked for native Wails acceptance; browser/design and automated checks passed.**
