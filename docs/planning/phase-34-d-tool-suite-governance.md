# Phase 34 D — Resource-owned Tool Suite bindings and typed Browser boundary

- Status: **Implementation closed (2026-10-12)**; real managed Browser backend/packaged Desktop testing are explicitly outstanding
- Architecture: [ADR-0039](../architecture/adr/0039-offerings-resource-tool-suites.md)
- Prerequisites: Phase 34 A/B/C committed; no new Agent loop or speculative browser backend

## Audited baseline

- `sandbox/domain.Capabilities` and `sandbox/application.WorkspaceSession` expose only `file.read/search/edit` and `process.shell`. `sandbox/pluginregistry.Registry.ResolveBackend` validates a host-registered, exact pinned backend and a required capability without opening the backend.
- A Workspace Provider already persists `{"sandboxId":"sbx_..."}` (ConfigVersion 2) and publishes canonical `workspace.*` Tools through Provider Activation and governed Invocation; Sandbox/Provider/MCP ToolIDs and Profile ToolBindings must not change.
- `agent_tool_facades` already records one Agent Resource→internal canonical Provider relationship with conflict detection, explicit ensure, and no automatic grant.
- No managed Browser Session backend, browser lifecycle/lease, approved browser tool schemas or isolation acceptance currently exists. A Browser-only environment cannot truthfully implement `WorkspaceSession`.

## Decision: host-owned Tool Suite definitions + exact resource binding

A Tool Suite is **not** a new native Plugin Extension protocol or an Agent Skill. Build a small host-owned, inert catalogue of reviewed definitions:
- `agent.delegation`: Agent resource, `agent.delegate/status/result/cancel`; executes through the existing Agent facade; AgentGrant + Policy remain separate.
- `sandbox.workspace`: Sandbox resource, requires **all** `file.read`, `file.search`, `file.edit`, `process.shell`, with stable `workspace.read/search/edit/shell` tools; only verified capable Backend drivers may bind.
- `sandbox.browser`: Sandbox resource requiring `browser.session` with a separate typed Browser Session interface. **Unavailable** until a real, independently admitted Browser backend and schemas are implemented and proven. No `navigate/click` Tool advertised as runnable.

Definition lists and resource association reads must be inert: no process launch, backend connection, remote API, credentials, filesystem mutation, implicit adoption or Provider creation. A suite's declared capabilities are requirements, **not proof of runtime support**.

For Sandbox Workspace, an explicit owner operation `EnsureSandboxWorkspaceTools(sandboxID)` reads the actual **exactly pinned backend** capabilities, checks Sandbox active state, and creates/adopts one canonical Workspace Provider. On successful sync it claims `(SandboxID, SuiteID, ProviderID)` in one authoritative SQLite binding table with unique resource/suite and ProviderID constraints. Repeating is idempotent. A previously created unique Workspace Provider is adopted in place; multiple existing Providers for one Sandbox yield `conflict`, never merge. A half-synchronized Provider yields a visible `incomplete` recovery state. New UI uses a focused **Tools** view in Sandbox details; there is no extra required Provider form.

Binding administration uses explicit local Owner Catalog context; never derives target ResourceID from Tool/model arguments, and never adds a Profile binding, AgentGrant, Sandbox grant or Policy allow. Destructive Provider CRUD cannot delete/rebind an owned binding. Sandbox destruction remains blocked by dependent Workspace Providers or leases, including under concurrent writers. Do not silently fall back to a local host Workspace for an unsupported backend.

Browser Session is a distinct consuming-side port, with browser-specific operations and typed isolation/session identity requirements. No registration, execution route, GUI Add, fake readiness or Process/Filesystem fallback is allowed until a real implementation meets admission/lease/permissions/secret-boundary gates. Existing Browser *Remote MCP* integrations remain plain MCP connections, not managed Sandboxes.

## API / UI / contracts

- `GET /tool-suites`: reviewed metadata with status/reason; no package installation or permissions.
- `GET /sandboxes/{sandboxId}/tools`: inspected Workspace Suite state (`missing/ready/legacy/incomplete/conflict/incompatible`), exact ProviderID and stable canonical tools, plus capability/availability diagnostics.
- `POST /sandboxes/{sandboxId}/tools`: explicitly ensure the Workspace Suite, with exact Owner and Provider Activation governance checks; fail closed when unsupported/unavailable.
- `DELETE /sandboxes/{sandboxId}/tools`: owner-authorized retirement only after Provider is proven unreferenced; generic Provider CRUD cannot tear down an owned suite.
- SDK, Wails/Desktop, Go CLI and React UI use the same shapes and semantics. Keep technical IDs in detail-only views. Present Browser suite as an inert `unavailable` reviewed definition, not installable or executable.
- Existing Agent Suite API remains unchanged and is included in generic reviewed suite definitions.

## Stage D checks (blocking)

1. Backend/Domain: capability lattice rejects Browser-only→Workspace, file-only→complete Workspace, Workspace-only→Browser; no host fallback or fabricated `browser.session`.
2. Binding: two capable Sandboxes get independent canonical tools and stable ProviderIDs on repeated ensure; no Profile exposure; existing unique Provider adoption preserves ToolIDs/export namespaces; duplicate Providers conflict and are never removed; forbidden caller causes zero side effects; unsupported/unready backend causes zero Provider creation.
3. Persistence: forward-only schema upgrade, uniqueness across concurrent Runtime processes, deletion/rebind guards and restart stability; existing SourceKeys/ToolIDs/Profile aliases unaffected.
4. HTTP/Wails/SDK/CLI: resource-level read/ensure and definition listings with parity, no secrets, no process/network on GET; POST validates exact SandboxID/owner and Policy admission.
5. React: one Sandbox detail Tools view lists its Resource-bound Workspace tools, explicit repair action and honest unsupported status; browser suite never shows a runnable Add.
6. Full `pnpm check`, `pnpm security:check` and `git diff --check` plus suite-specific negative tests. Manual real Browser operator smoke is **not** claimed without a registered backend.

### Commit discipline

Commit this contract **before** implementation, then backend/domain + persistence, HTTP/SDK/Wails/CLI, focused UI/tests, closure evidence. Keep stages bounded (D, not D1–D100). Do not include `.agents/`, `.pi/`, `.intent-loop/` or real credentials/resources.

## Implementation closure evidence

| Outcome | Commit | Verification |
| --- | --- | --- |
| Docs first and exact resource/capability constraints | `e32ac3a` | Accepted before code |
| Built-in Agent/Workspace/Browser Suite metadata, unavailable Browser Session port; SQLite v12 resource-owned Workspace mappings | `744a31f` | Typed capability denial, unique concurrent claims, legacy Provider/ToolID adoption, upgrade tests |
| HTTP/OpenAPI/SDK/Wails/CLI resource-level read, ensure and retire operations | `12b16a8` | Fixture HTTP/Desktop/CLI parity; no automatic Profile exposure |
| Focused Sandbox detail Tools view and Marketplace Suite presentation | `c0dd8b5` | React tests and bilingual i18n |
| Cached backend readiness, exact resource-Owner denial and restart-stable canonical Tool identity | `0bcdc71` | Go negative and restart tests |
| Unrelated Tunnel polling-test timing isolation | `afc0cdd` | No Tunnel runtime changes; test-only race fix |

**Full automated gates:** `pnpm check` **exit 0** (Go lint/tests/race, OpenAPI + Wails generator parity, strict TypeScript, ESLint, i18n, 30 Vitest files / 188 tests, Web/Desktop builds); `pnpm security:check` **exit 0**. Build emitted non-blocking large-chunk warnings.

**Limits explicitly preserved:** no real managed Browser Session driver, no operational browser Tool facade or fabricated security claims, no real provider/cloud credentials used, and no packaged Wails or live Browser acceptance. Third-party browser Remote MCP remains an ordinary MCP connection rather than an implicit Workspace Sandbox.
