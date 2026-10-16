# Phase 39 — Wails native acceptance and owner projection repair

Status: In progress · 2026-10-16
Parent: [Phase 39](phase-39-tools-workspace.md), [ADR-0046](../architecture/adr/0046-unified-tools-workspace.md)

## Objective

Accept the **already running, rebuilt macOS Wails window** against the user's real (read-only) Mender data. Close a real parity defect discovered by native accessibility inspection. Preserve all user-owned Resources, Credential references, ProviderID/ToolID, Profile bindings and Policies. No remote invocation or resource CRUD during acceptance.

## Read-only baseline

- Rebuilt Wails dev process launched at 16:01 on Oct 16; live window is titled `Mender`. The real SQLite database at `~/Library/Application Support/Mender/mender.db` has Catalog migrations v1 and v2, the `catalog_tool_revision` row and three revision triggers. `PRAGMA integrity_check` is `ok`, foreign-key violations 0.
- Live inventories at this inspection: 9 configured Providers, 29 canonical Tools, 21 Profile Tool bindings, 6 Sandboxes, 2 Agents, 1 Profile.
- Native macOS Accessibility tree confirms one sidebar **工具** entry, 9 service cards and **no internal/external Provider toggle**. Two separately registered AnySearch services each expose 4 Tools. Opening the first AnySearch card shows only its 4 canonical search/extract tools and a native **返回服务列表** action.
- **Confirmed defect:** The Go desktop service and generated Wails type expose exact `toolSourceKind`, `toolSourceId` and `toolSourceName`, and persisted Agent facade bindings map to the existing names `Grok - executor` and `Codex - Excutor`. However the TS `desktop-client.ts` `provider()` adapter drops those three fields before React receives them. Native UI therefore renders two generic `Agent Delegation` cards with **详情** instead of true owner-name **管理**, despite the HTTP projection being correct.
- The existing Workspace `Mender` facade has no exact owner binding; do not infer an owner from a label, config JSON, or matching Sandbox names.

## Repair/acceptance contract

1. Preserve and validate exact typed Wails Host owner fields in `provider()`. Optional absent values remain absent, and an unknown nonempty owner kind fails closed. No API/schema/DB duplication, no compatibility bridge.
2. Add Wails adapter tests for direct Provider, Agent, Sandbox and unclaimed output, plus unknown owner-kind rejection. Keep existing nullable array handling intact.
3. Rebuild/typecheck/test and verify *in the native WKWebView window* that Agent cards display canonical Agent names and the `Manage` action navigates to Agent details; same for Rocky Workspace/Jobs to Sandbox detail. AnySearch remains name-first and its Tool drilldown remains scoped.
4. Verify Plugin Marketplace `AnySearch` Connect preview without connecting; inspect Profile Tool selection without saving; no tool calls, no installation/deletion, no authorizations.
5. Record executed gates and real constraints before closing. Stage docs first, code fix second, evidence last; exclude `.agents/`, `.pi/`, `.intent-loop/`.
