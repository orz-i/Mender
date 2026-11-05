# Mender Agent catalog as presets — Magpie-style UX (2026-11-05)

## Verified model
- Mender defines four built-in Agent **backend presets** in `agentBackendDetails`: Codex, Grok, OpenCode, Antigravity.
- `listPluginCatalogue()` reports built-in backend health/status and whether creation is permitted.
- `listAgents()` returns **configured Agent resources** (workspace-bound), not a complete built-in catalog. Multiple configured resources may share one backend. An unconfigured preset must not be shown as a configured Agent or an available CLI merely because it appears in the catalog.
- The existing Agent execution, Task, Profile grants and approvals remain tied to the configured Agent ID. Preserve all backend IDs/Agent IDs and lifecycle semantics.

## User expectation / selected reference
Match Magpie's Agents screen shown by the user: one full-width bordered list, each 70–86px row with a distinct Agent icon, strong backend name, concise relevant description and **inline status**. A row navigates to configuration. Remove the status tabs, search input and "Add agent" inventory action. Keep keyboard navigation, responsive layout, unavailable diagnostics and honest load/failure states.

## Navigation and status
- Show the four **preconfigured backend presets**, even if no Agent resource has been configured yet. Data source: exhaustive built-in registry, enriched by host plugin catalogue and configured instances.
- One configured resource for backend → clicking enters existing `/agents/:agentId` details and configuration (no creation).
- Zero configured resources & host reports enabled, healthy plugin → clicking enters pinned `/agents/new?pluginId=...` *configuration*, not a generic backend selector.
- Two or more existing configurations → clicking enters an explicit backend workspace selection page; do not silently pick the first or overwrite an Agent.
- Missing, unavailable or disabled plugin → clicking enters a backend status/diagnostics page; do not offer a fake enabled setup, and never silently pick a different backend.
- Plugin catalogue unavailable → distinguish **verification unavailable** from genuinely unavailable, retain configured resource navigation and show a non-blocking error with Retry.
- Inline statuses distinguish: configured and CLI detected, configured but CLI missing, not configured with healthy preset, preset unavailable, or status unknown. Do not mislabel "configured" as "connected" or "running".
- All existing Profile/approval/tool/task operations remain on configured Agent detail. Keep the internal `createAgent` operation for first workspace setup, but user-facing language is **Configure / Save configuration**, not "Add agent"; no second backend selection for pinned routes.
- Marketplace remains host-authoritative about which native preset can be configured. No backend/schema/ACP changes, and no migrations/compat bridges.

## QA acceptance
- At least 4 preset rows when Agent inventory empty; include unavailable backend and corresponding explanation.
- No inventory status tabs, search or add button. Each row has distinct icon, description, inline status and a deterministic destination.
- Verify exactly one / multiple / zero configured variants, plugin unavailable and catalogue error independently.
- Retain underlying Agent creation validation, pinning, review and Task/approval flows.
- En/zh-CN, keyboard, desktop/mobile widths, Wails/browser render, Vitest, lint, tsc, production frontend builds.
- Do not write real user's Agent store during QA; mock API for flow tests. Changes committed in stages, not pushed.

## Progress
- [x] Preset catalog and backend selection
- [x] Pinned configuration language and marketplace CTA
- [ ] Responsive/browser/Wails QA and full gates — browser, axe, Vitest, lint, TS, translations, Web/desktop frontend build passed; native Wails window unavailable for post-change visual acceptance

## Acceptance evidence

[Agent preset directory design and browser QA](../design/agent-presets-qa/README.md) documents desktop/mobile captures, direct existing-Agent configuration, multi-workspace selection, unavailable diagnostics and all limitations. Brand marks are locally vendored from MIT-licensed LobeHub Icons with source/notice files. No production Agent inventory, grants, Task histories or plugin status were modified.
