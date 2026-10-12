# ADR-0039: Govern offerings, configured resources, and bound tool suites separately

- Status: Accepted for Phase 34
- Date: 2026-10-12
- Supersedes: none; refines ADR-0035, ADR-0037, and ADR-0038 without replacing their security invariants

## Context and inspected baseline

Mender is a governed Tool/Resource runtime for external LLM/Agent clients, **not** an Agent loop. ADR-0037 already separates version-pinned Plugin Packages, typed Extensions, and configured Resources; ADR-0038 retains a host-owned Agent Delegation Provider because the Catalog and Invocation pipeline require a Provider identity. The product presently surfaces that internal distinction as repetitive manual work.

Current code evidence:

- `backend/internal/agent/infrastructure/delegationprovider/plugin.go`: one configured Provider binds one AgentID, exposes four identical `agent.delegate/status/result/cancel` SourceKeys, and guards cross-Agent TaskID access.
- `backend/internal/agent/application/service.go` and `backend/internal/management/httpapi/agents.go`: creating an Agent persists only an Agent. The user must separately create/sync a Delegation Provider and grant a Profile.
- `backend/internal/mcp/infrastructure/server/server.go`: exported direct-tool names are derived from Provider Namespace and Profile presentation aliases, not from the underlying Agent name. Existing client-facing names must not change silently.
- `backend/internal/sandbox/domain/sandbox.go` and `backend/internal/sandbox/application/ports.go`: the current execution capability enum/session is specifically Workspace `file.read/search/edit/process.shell`, not a universal browser session.
- `backend/internal/bootstrap/plugin_catalogue.go`, `packages/app/src/domains/plugins/marketplace.ts`: the Marketplace projects technical Extension descriptors; it has no first-class provider-preconfigured Remote MCP offering.
- Agent-to-Provider deletion reference protection is enforced at SQLite commit; Profile ToolBinding, AgentGrant, Sandbox leases, Provider Activation, Runtime Budget, Credential refs, Policy and Audit are separate safeguards.

## Decision: five distinct responsibilities, not five mandatory configuration steps

1. **Marketplace Offering** is a signed/reviewed or explicitly trusted discovery/configuration recipe: e.g. AnySearch Remote MCP, Grok Agent, Browser Suite. An offering may point to a built-in driver and need **no separate executable package**. An offering is not itself a live Resource, Tool, trusted endpoint, or permission grant.
2. **Plugin Package / typed Extension** implements a versioned driver contract (`provider.tools`, `agent.backend`, `sandbox.backend`, `tunnel.transport`). Preserve exact package/installation pinning, host-supervised processes, and type-specific lifecycle/authority. Do not introduce an oversized universal Extension service.
3. **Configured Resource** has a stable identity, settings, ownership, Credentials refs, lifecycle, and observed health. A Remote MCP connection continues using existing Provider configuration internally. Agent, Sandbox, and Tunnel retain their distinct bounded contexts.
4. **Tool Suite Definition + Binding** identifies a coherent set of canonical tool definitions, required operation capabilities, the exact resource binding, execution adapter, and governance requirements. The host may supply built-in suites (Agent Delegation, Workspace, Browser). It must never infer file/shell capabilities for a browser. Do not conflate Tool Suites (executable operations) with Agent Skills (instructions and resources).
5. **Tool Exposure** is the existing Catalog/ToolID, Profile ToolBinding, governed Invocation, MCP/CLI/Skill projection and Policy/Grant/Audit chain. Discovery/creation/binding alone never authorizes a Profile or exposes credentials.

A Tool Suite is initially a **host-owned declarative definition/binding**, not a fifth native plugin protocol. Build a new executable extension protocol only when an independently implemented external suite needs it. One suite may bind several independent resource instances; each ResourceID remains unforgeable and each resulting ToolID stable.

## Internal Provider remains a canonical execution identity

Retain the existing `catalog.Provider`, `provider.tools` extension, `ProviderID`, namespace/SourceKeys, Activation and Invocation Router as execution infrastructure. **Do not require the user to create a second visible Provider** to operate a configured Agent or Sandbox.

Introduce a host-owned *managed tool facade* relation for resource-backed suites. For Agent Delegation the existing built-in plugin and its `agent.*` SourceKeys remain the sole executor; resource creation/configuration may explicitly provision and sync its facade without granting any Profile. A managed facade records its exact AgentID, SuiteID, ProviderID and lifecycle owner. Do not use display names as identities or let Tool arguments select AgentID/SandboxID.

A managed facade is not an independently configurable connection. Ordinary Provider management can keep machine-facing compatibility but the UI must not offer duplicate create/edit/delete pathways that bypass resource ownership. An explicit owner operation must remain available to repair a partially configured facade. **Read-only Agent/Marketplace/Catalog lists must not create, repair or activate resources.**

Migration/adoption:

- Preserve all existing AgentIDs, ProviderIDs, ToolIDs, SourceKeys, namespaces, Profile ToolBindings, alias overrides, Policy references and task history. Do not rename existing exported MCP tool names.
- Adopt an existing Provider into a managed facade **only when** its exact typed configuration points to an unambiguous AgentID and the owner verifies it. If multiple Providers already reference one Agent, **do not merge, delete, or silently rebind**; surface a conflict and leave working exposure untouched until explicit resolution. A missing facade is visible and recoverable with an owner-authorized operation.
- New facade creation must be idempotent at the persistence/ownership boundary; prevent concurrent duplicates. If cross-context create/sync cannot commit atomically, record actionable incomplete state and fail closed rather than silently grant or replay work.
- Deleting an Agent must be blocked by existing Profile ToolBindings/Policy/history/active tasks and facade usage until explicitly drained; do not automatically cascade existing Tools or remove exposure without a reviewed operation.
- No automatic Profile assignment, AgentGrant, Policy allow, secrets resolution or CLI/model invocation. Existing normal and admin APIs retain their security boundaries.

## Integration offerings and Marketplace

Keep the existing unified native *installation inventory* as a separate technical concern. The user-facing Marketplace may combine reviewed Integration Definitions and native extensions, but must distinguish `offeringId` from `pluginId`, `installationId`, configured `resourceId`, and `toolId`. A built-in MCP driver appears as an Advanced/Custom connection foundation, not as a competing AnySearch product.

A Remote MCP Integration Definition pins the built-in MCP driver, transport and vendor-published endpoint, with bounded reviewed user-configurable fields (display name, optional Credential Reference). It contains no executable code or secret bytes. The AnySearch preset is a reviewed, replaceable catalog item, **not a claim that authentication/connectivity is healthy**. User-supplied endpoints remain available through Advanced MCP connection creation.

Marketplace actions are typed: Connect (Remote MCP), Create (Agent/Sandbox), Bind (Tool Suite), Install (native verified package). An item may have separate package-install status, configured-resource status and Profile-exposure status. All creation flows remain focused, schema-reviewed, localized and one information category per viewport.

Registry metadata and third-party descriptions are untrusted; do not download native packages, start sessions, resolve Credentials or expose tools on catalogue reads. Newly discovered or materially changed Tool schemas are **not automatically authorized** to Profiles.

## Browser and other capability families

`WorkspaceSession` remains valid for compatible file/process Sandboxes. Add separate capability contracts (e.g., `browser.session`) and consumer-owned session ports only with a real backend; no universal `read/edit/shell` pretending browser navigation, screenshots or DOM access are files. Host governs session ownership, tenant isolation, navigation/network restrictions, cookie/credential boundaries, lease, cancel, lifecycle and audit. Unsupported operations fail closed; no fallback to host shell.

When a cloud browser publishes its own Remote MCP Tools, connect it as Remote MCP rather than inventing a Sandbox. Create a Browser Sandbox/Browser Tool Suite only when Mender actually controls the browser session lifecycle and provides verified browser operations. Sandbox isolation labels remain truthful; browser tenancy does not imply VM isolation.

## Governance order and security

Every new tool invocation remains: trusted Principal/Scope + Profile ToolBinding → argument/schema validation → reviewed resource/suite facts → Policy + approval → runtime budget / binding guard → resource-specific grant/admission/lease → exact executor → result guard + minimized audit. Host owns Config/Credential refs and package trust. Hidden/internal facades do not bypass `provider.activate`, `agent.task.submit`, `sandbox.acquire` or any existing denial. Do not allow third-party definitions to inject React or executable code into the UI.

## Non-goals and trade-offs

No Mender LLM reasoning loop, generic multi-Agent orchestration, recursive tool exposure, auto-run of external Agent accounts, universal Sandbox shell emulation, hidden automatic publishing, OCI registry implementation or speculative new native extension kinds. Canonical Provider plumbing survives internally for reliable ToolID, Policy and SDK compatibility, which makes migration more involved than simply deleting a UI page.

## References

- [ADR-0035: Sandbox router](0035-governed-sandbox-execution-router.md)
- [ADR-0037: Typed plugin platform](0037-multi-kind-plugin-platform.md)
- [ADR-0038: Agent Delegation](0038-governed-agent-delegation.md)
- https://modelcontextprotocol.io/registry/about
- https://modelcontextprotocol.io/specification/2026-07-28/basic/utilities/tasks
- https://github.com/microsoft/playwright-mcp
- https://github.com/browserbase/mcp-server-browserbase
