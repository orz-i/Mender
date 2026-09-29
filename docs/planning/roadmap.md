# Roadmap

This roadmap is ordered by architectural dependency, not by UI visibility.

A phase is complete only when its exit criteria are met. Work from a later phase may be prototyped, but it should not force unreviewed architectural decisions into earlier layers.

## Phase 0 — Governance and architecture baseline

Status: Complete

Goals:

- Establish product scope and boundaries.
- Record architecture principles.
- Accept foundational ADRs.
- Define contribution and change-governance rules.
- Define the implementation roadmap.

Exit criteria:

- Product vision is committed.
- Architecture principles are committed.
- Foundational ADRs are accepted.
- Development workflow and Definition of Done are committed.
- Phase 1 scaffold plan is explicit.

## Phase 1 — Foundation scaffold

Status: Complete

Goals:

- Create the monorepo/workspace skeleton.
- Establish repeatable local commands.
- Install static quality gates before feature implementation.
- Prove backend/frontend/desktop boundaries without implementing product features.

Planned deliverables:

- Go workspace/module layout.
- pnpm workspace layout.
- `apps/web`.
- shared frontend packages.
- backend application entry points.
- strict shared TypeScript configuration.
- repository-pinned golangci-lint configuration.
- ESLint/typecheck/build commands.
- Taskfile or equivalent repository command facade.
- baseline CI.
- architecture dependency checks where practical.
- minimal test harnesses.

Exit criteria:

- Fresh checkout can install/build/test through documented commands.
- Go lint and test pass.
- TypeScript lint, strict typecheck, test, and build pass.
- No product domain package imports infrastructure/framework SDKs.
- Web shell renders without containing product-domain implementation.
- Wails remains optional and does not block server/CLI builds.

See `phase-1-foundation.md`.

## Phase 2 — Catalog domain and persistence

Status: Complete

Goals:

- Establish canonical Provider and Tool models.
- Implement provider registration and catalog synchronization abstractions.
- Add SQLite persistence behind context-owned ports.
- Add initial tool search.

Exit criteria:

- Canonical Tool model has no MCP SDK dependency.
- Provider identity and Tool identity rules are tested.
- Tool schemas round-trip through persistence.
- Search/describe queries work through application use cases.
- Catalog infrastructure has integration tests.

See `phase-2-catalog.md`.

## Phase 3 — Invocation, policy, credentials, and audit

Status: Complete

Goals:

- Implement the shared invocation pipeline.
- Define policy decisions and approval hooks.
- Resolve credential references only at the execution boundary.
- Record audit lifecycle safely.

Exit criteria:

- One fake/built-in provider executes through the complete pipeline.
- Denied policy prevents execution.
- Secret material does not appear in domain events or normal audit records.
- Timeout/cancellation behavior is tested.
- Invocation model can represent immediate result, input-required, and task-oriented outcomes.

See `phase-3-runtime.md`.

## Phase 4 — MCP provider and server adapters

Status: Complete

Goals:

- Consume local stdio and remote MCP providers.
- Normalize MCP tools into the canonical catalog.
- Expose profiles through MCP.
- Support local stdio and remote Streamable HTTP entry points.

Exit criteria:

- One upstream MCP tool can be synchronized and invoked.
- The same tool can be invoked through CLI/application and Mender MCP server paths.
- All paths use the same invocation pipeline.
- Profile exposure rules are enforced.
- MCP SDK types do not leak into domain packages.

See `phase-4-mcp.md`.

## Phase 5 — CLI discovery and Agent Skill

Status: Complete

Goals:

- Make local mode independently useful.
- Support search, describe, invoke, provider, and profile workflows.
- Provide progressive disclosure through an Agent Skill.

Exit criteria:

- A user can configure and operate Mender without Web/Desktop.
- Large catalogs can be used without eagerly loading every tool schema.
- Skill instructions rely on stable machine-readable CLI output.
- CLI commands have integration coverage.

See `phase-5-cli-skill.md`.

## Phase 6 — Management Web UI

Status: Complete

Goals:

- Build management surfaces for providers, catalog, profiles, policies, credentials, and audit history.
- Reuse generated/typed contracts.
- Keep host transport behind frontend adapters.

Exit criteria:

- Core administration workflows are available in browser.
- Strict TypeScript checks remain enabled.
- UI domain language matches backend ubiquitous language.
- No direct scattered `fetch` usage bypasses the SDK/API boundary.

See `phase-6-management-web.md`.

## Phase 7 — Wails desktop distribution

Status: Complete

Goals:

- Package the shared management UI as a native desktop application.
- Add desktop-specific local integrations only where they provide value.

Exit criteria:

- Desktop uses the same core application semantics as CLI/server.
- Wails-specific code stays at the shell/binding boundary.
- Server and CLI build/test without desktop dependencies.

See `phase-7-wails-desktop.md`.

## Phase 8 — Hardening and distribution

Status: Complete

Goals:

- Sandboxing and stronger provider trust controls.
- Release signing and checksums.
- SBOM and dependency/security scanning.
- MCPB/package compatibility where justified.
- Upgrade/migration strategy.
- Performance and catalog-scale validation.

Exit criteria are complete. See `phase-8-hardening-distribution.md` for the hardening decisions, release policy, verification evidence, and deferred platform-specific sandbox/signing work.

## Phase 9 — Governed invocation execution

Status: Complete

Goals:

- Make execution scope and origin explicit for every Invocation.
- Enforce Profile membership inside the shared Invocation pipeline.
- Validate Tool arguments against canonical input schemas before Policy/execution.
- Provide bounded Governance Facts instead of unrestricted raw arguments to Policy.
- Represent expected rejection/failure conditions as canonical Invocation outcomes.
- Add execution/result guards and data-minimized lifecycle events.
- Preserve one adapter-independent execution contract across CLI, MCP, Web, and Desktop.

Exit criteria are complete. Profile/Catalog execution scopes, input/output validation, canonical rejection/failure outcomes, execution/result guards, lifecycle events, governed Audit metadata, and cross-adapter mappings are covered by the shared Runtime and end-to-end tests.

See `phase-9-governed-invocation.md` for the final architecture, validation evidence, and deferred governance work.

## Phase 10 — Profile ToolBinding governance

Status: Complete

Goals:

- Replace Profile Tool membership selections with first-class ToolBindings.
- Add Profile-specific alias/title/description presentation without mutating Catalog.
- Let a binding tighten global Policy with inherit/require_approval/deny semantics.
- Add binding-local serial execution on ProfileID + ToolID while preserving the Runtime baseline ExecutionGuard.
- Keep membership selection separate from presentation/execution governance in Web/Desktop UX.
- Preserve existing Profile rows and clients through behavior-compatible defaults.

Exit criteria are complete. ToolBinding is now the canonical Profile-to-Tool relationship, Profile SQLite v2 preserves v1 behavior, binding policy only tightens global Policy, binding-local serial execution is enforced inside Invocation, MCP uses effective Profile presentation without mutating Catalog identity/schemas, and all adapters/UI share one normalized binding contract without manual ToolID entry.

See `phase-10-profile-tool-binding-governance.md` for implementation evidence and deferred Credential/global-Policy work.

## Phase 11 — Credential inventory and scoped Provider authentication

Status: Complete

Goals:

- Introduce a real non-secret Credential inventory.
- Resolve native secrets from the OS keyring and headless secrets from environment-backed sources without plaintext fallback.
- Add inventory-validated CredentialRefs to Profile ToolBindings.
- Restrict Profile-scoped executor Credential resolution to the selected allowlist while Catalog scope remains explicitly privileged.
- Let MCP Provider HTTP headers and stdio environment entries bind to Credential References instead of persisted secret values.
- Prevent a Profile from borrowing a shared authenticated Provider connection without access to the Provider-required Credential References.
- Replace the read-only Credentials UI with list/create/rotate/delete workflows and inventory selectors.

Exit criteria are complete. Credential inventory metadata is persistent while secret values remain in native keyring/environment sources, Profile scope enforces an inventory-backed Credential allowlist, authenticated Provider connections re-authorize required References on every Tool call, and Web/Desktop use inventory selectors without raw Reference entry.

Legacy environment header templates remain supported as an explicitly labeled compatibility exception rather than inventory-scoped authentication.

See `phase-11-credential-governance.md` for implementation evidence and deferred session-pool/cloud-secret work.

## Phase 12 — Persistent Policy governance

Status: Complete

Goals:

- Replace the local AllowAll placeholder with one persistent Runtime-wide PolicySet.
- Add an editable default decision plus unordered allow/require-approval/deny rules.
- Match stable origin/scope/Profile/Provider/Tool metadata and bounded Governance Facts without raw arguments or secrets.
- Keep evaluation deterministic: no priority/order semantics, deny dominates matching approval/allow rules, and the default applies only when no rule matches.
- Preserve Profile ToolBinding policy as a second tightening-only layer.
- Replace the read-only Policy page with list-first Rule management and inventory-backed selectors.

Exit criteria are complete. Fresh migration preserves current allow behavior; default-deny allowlists, deny-over-allow composition, explicit unknown-fact rules, stable Audit reasons, Policy SQLite, CLI/OpenAPI/Wails, and Web/Desktop management are covered by the shared Runtime and tests. `pnpm check` and `pnpm security:check` are green.

See `phase-12-policy-governance.md` for implementation and verification evidence.

## Phase 13 — Provider Plugin Runtime and MCP unification

Status: Complete

Goals:

- Make Provider plugins the runtime/configuration boundary while preserving canonical Provider and Tool models.
- Treat upstream MCP as one built-in plugin whose stdio and Streamable HTTP forms are transport variants.
- Introduce deterministic Provider instance reuse, reload, replacement, and shutdown instead of implicit executor/closer ownership.
- Replace MCP-specific Provider configuration/orchestration with generic plugin configuration and registry lookup.
- Preserve ProviderID, ToolID, Profile ToolBindings, Policy targets, Credential References, and Audit history through migration.
- Migrate CLI/OpenAPI/Wails/Web/Desktop management to plugin-oriented Provider semantics.
- Leave a stable extension boundary for OpenAPI and model-hosted Tool Providers without implementing them in this phase.

Exit criteria are complete. Generic plugin configuration is persistent and imported from the legacy MCP schema without changing canonical identities; `io.mender.provider.mcp` owns both stdio and Streamable HTTP; Provider instances are reused/replaced/closed deterministically with Provider-scoped synchronization; Credential usage inspection works through the plugin boundary; CLI/OpenAPI/Wails/Web/Desktop use plugin-oriented Provider contracts; and all repository/security checks are green.

See `phase-13-provider-plugin-runtime.md` for implementation and verification evidence.

## Phase 14 — OpenAPI Provider Plugin

Status: Complete

Goals:

- Add `io.mender.provider.openapi` as the first non-MCP Provider plugin.
- Convert supported OpenAPI 3.0/3.1/3.2 operations into canonical Mender Tools with stable SourceKeys and self-contained JSON Schemas.
- Execute JSON HTTP operations through the shared governed Invocation pipeline.
- Bind supported OpenAPI Security Schemes to Mender Credential References without exposing secrets as Tool arguments.
- Reject external references and unsupported media/serialization/authentication semantics rather than silently weakening execution.
- Reuse generic Provider configuration, lifecycle, CLI/API/Wails/Web/Desktop plugin surfaces without adding OpenAPI branches to Catalog/Invocation Runtime orchestration.

Exit criteria are complete. `io.mender.provider.openapi` loads bounded URL/file OpenAPI 3.0/3.1/3.2 descriptions with external refs disabled; discovers deterministic canonical Tools with self-contained schemas; executes bounded JSON HTTP operations; supports apiKey/Bearer Security Schemes through scoped Credential References; preserves ToolID across SourceKey-stable refreshes; uses the generic Provider lifecycle/management surfaces; and passes repository race/build/security verification.

See `phase-14-openapi-provider.md` for implementation and verification evidence.

## Phase 15 — Model Tool Bridge Provider

Status: Complete

Goals:

- Add `io.mender.provider.model-tools` for provider-hosted tools executed inside model API agentic loops.
- Support OpenAI, Anthropic, and Gemini engine adapters behind one Provider plugin.
- Normalize the common v1 `web_search` capability into one stable Mender Tool schema/result contract.
- Verify that the provider-hosted search tool actually executed before accepting a successful result.
- Preserve citations while excluding reasoning/thought/internal provider state from canonical results and Audit.
- Resolve model API Credentials only during governed execution so Profile Credential scope blocks network access before the provider request.
- Reuse generic Provider configuration/lifecycle/CLI/API/Wails/Web/Desktop surfaces without engine-specific branches in Catalog/Invocation Runtime orchestration.

Exit criteria are complete. `io.mender.provider.model-tools` normalizes OpenAI, Anthropic, and Gemini provider-hosted web search into one stable Mender Tool contract; verifies observable hosted-search execution; preserves citations while discarding reasoning/internal provider state; resolves API Credentials only during governed execution; enforces Profile Credential scope before network access; uses generic Provider lifecycle/management surfaces; and passes repository race/build/security verification.

See `phase-15-model-tool-bridge.md` for implementation and verification evidence.

## Phase 16 — External Provider Plugin Host

Status: Complete

Goals:

- Let users install Provider plugin types without recompiling Mender.
- Keep external plugin code out of the Mender process behind a versioned protobuf/gRPC protocol.
- Separate PluginID, package version, protocol version, config version, and installation identity.
- Pin external Providers to exact managed plugin installations and package digests.
- Preserve host-owned Invocation, Policy, Profile, Credential, and Audit governance.
- Render unknown external Provider configuration through Mender-owned JSON-Schema-driven management UI instead of PluginID-specific frontend branches.
- Provide a public Go SDK and standalone example plugin as the architectural acceptance test.

Exit criteria are complete. An independently built external Provider can be packaged, installed into the managed plugin root, discovered/configured through declarative schema metadata, synchronized into the canonical Catalog, invoked through the shared governance pipeline, disabled/enabled, and removed without adding its PluginID to Mender source code. Exact InstallationID pinning prevents silent package retargeting; CLI/OpenAPI/Wails/Web/Desktop management, the public Go SDK/example, process/package/Credential hardening, `pnpm check`, and `pnpm security:check` are all verified.

See `phase-16-external-provider-plugin-host.md`.

## Phase 17 — Provider Plugin Lifecycle & Runtime Resilience

Status: Complete

Goals:

- Recover configuration-identical external Provider instances after their plugin process exits, without replaying the failed invocation.
- Bound external plugin control-plane RPCs with host-owned deadlines.
- Persist verified external plugin descriptor snapshots so normal management discovery does not execute third-party code.
- Persist normalized Provider configuration metadata such as Credential References and connection summary so Mender-owned governance remains available while plugins are unavailable.
- Keep administrative enablement separate from runtime process/session health.
- Add an explicit same-PluginID installation rebind that preserves ProviderID and SourceKey-stable ToolIDs.
- Preserve exact InstallationID pinning: upgrades remain explicit and never follow latest automatically.

Exit criteria are complete. External plugin sessions recover on later requests after process failure without replaying failed invocations; control-plane RPCs are host-bounded; verified descriptor and normalized config metadata make management/Credential governance available without executing third-party code; and explicit same-PluginID rebind preserves ProviderID plus SourceKey-stable ToolIDs. CLI/OpenAPI/Wails/Web/Desktop expose the shared rebind lifecycle, and full repository/race/build/security verification is green.

See `phase-17-provider-plugin-resilience.md`.

## Phase 18 — Built-in Provider Plugin Package

Status: Complete

Goals:

- Make MCP, OpenAPI, and Model Tools one explicit statically linked built-in Provider plugin package at the composition boundary.
- Keep the generic Provider Plugin Runtime implementation-agnostic.
- Remove direct first-party plugin implementation imports from Runtime composition.
- Preserve all existing PluginID, ProviderID, ToolID, ConfigVersion, management, and external-plugin compatibility semantics.

Exit criteria are complete. `backend/internal/providerplugin/builtin` now owns the deterministic first-party plugin set and registration entrypoint; Runtime performs one built-in registration call and no longer imports MCP/OpenAPI/Model Tools implementations directly. Existing plugin identities/configuration and external installation semantics are unchanged. Focused and repository lint/test/race/typecheck/build gates pass; the only unavailable aggregate-check step was remote Buf code generation because the Buf hosted plugin service was unavailable, with no generated protocol diff.

See `phase-18-built-in-provider-plugin-package.md`.
