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

Status: Next

Goals:

- Package the shared management UI as a native desktop application.
- Add desktop-specific local integrations only where they provide value.

Exit criteria:

- Desktop uses the same core application semantics as CLI/server.
- Wails-specific code stays at the shell/binding boundary.
- Server and CLI build/test without desktop dependencies.

## Phase 8 — Hardening and distribution

Goals:

- Sandboxing and stronger provider trust controls.
- Release signing and checksums.
- SBOM and dependency/security scanning.
- MCPB/package compatibility where justified.
- Upgrade/migration strategy.
- Performance and catalog-scale validation.

Exit criteria are defined when this phase becomes active.
