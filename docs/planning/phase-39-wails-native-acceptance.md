# Phase 39 — Wails native acceptance and owner projection repair

Status: Native Wails dev-mode acceptance closed · 2026-10-16
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

## Executed acceptance evidence

### Actual macOS Wails host (not a browser substitute)

- `wails3 dev` and its `mender-desktop` WKWebView window were already running on the Mac mini; the rebuilt native process started at **16:01** on 2026-10-16. UI interactions were performed against `System Events`' **mender-desktop** native Accessibility tree and actual focused controls, not against the isolated Web/HTTP QA server.
- The sidebar has one **工具** item. The tool-services home lists **9 canonical sources** without internal/external toggles; each of the two user-configured AnySearch instances displays **4 Tools**. Following the first AnySearch link showed exactly `batch_search`, `extract`, `get_sub_domains`, `search` and **返回服务列表**. Opening `batch_search` displayed the real Tool detail with description, Schema and Invoke controls. **Invoke was not clicked**.
- Before repair, both Agent service cards appeared as generic **Agent Delegation** because `apps/desktop/frontend/src/desktop-client.ts` discarded Host `toolSourceKind/Id/Name`. The source data from the Wails Go host and existing persisted Agent/Sandbox binding tables were correct. After the frontend hot reload, cards displayed **Codex - Excutor**, **Grok - executor**, **Rocky · 工作区**, and **Rocky · 任务工具**, with owner **管理** rather than generic Provider editing controls. The Codex Manage action opened its native Agent details. Keyboard-targeted (exactly focused) Manage actions on **both Rocky Workspace and Rocky Jobs** independently opened the real **Rocky Sandbox** detail page. Identical button accessible names initially made naive name-only UI automation select the first Manage button; unique link focus + two Tab transitions + Enter resolved that test ambiguity.
- From the native Plugin Marketplace, AnySearch's **连接** dialog displayed the pre-reviewed endpoint `https://api.anysearch.com/mcp`, its Credential selector and the explicit no-auto-Profile-access copy; **Connect was not submitted**.
- Native Profile **X** Tool Picker displayed **29 available Tools and 21 already selected**. Its source dropdown contained actual service/Agent/Sandbox names. The legacy **提供方筛选 / 所有提供方** terminology was replaced and visibly verified after hot reload as **服务筛选 / 所有服务**. No Profile save or grant/revoke was performed.

### Live database and safety

- Read-only `PRAGMA integrity_check = ok`, `PRAGMA foreign_key_check = 0`. Catalog migrations **v1 and v2** are present; the `catalog_tool_revision` row and all three Tool-change triggers exist. The existing Wails process really had the live Mender database open.
- Observed inventory: **9 Providers, 29 Tools, 6 Sandboxes, 2 Agents, 1 Profile, 21 Profile Tool bindings, 2 Credentials**. No resource was created, re-synchronized, installed, executed, deleted or reauthorized by this acceptance.
- **One pre-existing Workspace facade** named `Mender` lacks a canonical Sandbox Tool Suite ownership binding; its read-only service card remains visible with Details, and the assistant deliberately **did not infer a Sandbox from matching names or mutate the binding**. This is an existing ownership/repair follow-up, not a failure of the owner projection. Agent task execution was not tested; the local Codex backend was shown as unavailable in one native detail view.

### Fix, tests, and release boundary

- **Docs first:** `754f05c` captured the native discrepancy, safety contract and intended test coverage before modifying code.
- **Repair:** `89a5345` passes Host-verified source kind/ID/name through the typed desktop adapter, rejects unsupported/incomplete resource ownership, adds Wails and Agent/Sandbox navigation tests, and removes residual Provider terminology from the Profile service filter and empty Tool-list help.
- Initial full `pnpm check` attempt failed at an SDK lint **because an unrelated parallel `golangci-lint` process held the tool lock** (exit 3); no source diagnostics were raised. After the competing process exited, a fresh full `pnpm check` completed with **exit code 0** (`/tmp/mender-phase39-wails-native-final/check2.log` and `check2.exit`): Go modules/SDK/protocol schema, Wails binding/schema drift, architecture, Go lint, complete Go tests and Race, ESLint, TypeScript, i18n, **41 files / 232 frontend tests**, and Web/Desktop production builds. Chinese i18n: **1200/1200**. Only existing large Vite JS chunk warnings remain.
- A subsequent `pnpm security:check` also completed with **exit code 0** (`/tmp/mender-phase39-wails-native-final/security.log`): no reachable Go vulnerabilities, no known production JS high-risk vulnerabilities. Govulncheck noted one required-module vulnerability whose vulnerable symbols were not called.
- **Boundary:** These are genuine macOS native **Wails dev-mode** UI checks, not a separately packaged release binary. Real AnySearch remote-service network reachability, tool invocation, Credentials, Agent execution and end-to-end external LLM/Profile authorization were intentionally not attempted. The running WKWebView consumed the final desktop frontend through Vite hot reload; subsequent packaged releases must include the current build.
