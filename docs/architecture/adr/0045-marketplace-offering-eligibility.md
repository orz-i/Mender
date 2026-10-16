# ADR-0045 — Marketplace offerings are actionable, not every registered capability

- Status: Accepted
- Date: 2026-10-15
- Refines: ADR-0037, ADR-0039, ADR-0040; no change to their execution/governance ownership

## Context

The current Plugin Marketplace combines the complete technical `ListPluginCatalogue` inventory, curated Remote MCP Integration Offerings and host-owned Tool Suite definitions. The application then guesses `Add` from an extension `kind`, hides the three user-configurable MCP/OpenAPI/Model Tools foundations behind a `Show foundation drivers` switch, and presents internal Workspace/Jobs/Agent facades, unavailable Browser tools and roadmap E2B as market cards. These are not equivalent product offerings. `ProviderPluginDescriptors` is also consumed by both internal resolution and the user-facing Provider chooser.

A technical extension may be required for ToolID/Provider activation while having no independently configurable user lifecycle. Likewise, a plugin installation may be disabled/degraded yet still need visible repair/removal controls, without qualifying as a discoverable executable offering.

## Decision

1. **Authoritative technical catalogue stays complete and inert.** The existing `PluginCatalogue` and one exact installation registry remain the facts for built-in/installed extensions, resource references, status and diagnostics. Do not create a shadow installation inventory or launch code, probe endpoints or resolve secrets on catalogue reads.
2. **Only reviewed actionable offerings enter Marketplace discovery.** The Host projects an explicit, non-plugin-controlled `marketplaceAction`: `create.provider`, `create.sandbox`, `create.agent`, `create.tunnel`, or `none`. Built-in identities are reviewed individually. A verified, supported native `provider.tools`/`sandbox.backend`/`tunnel.transport` installation gets its owning resource's action; unsupported kinds are never silently advertised as creatable. Mere descriptor `kind`, `capabilities`, `enabled`, or a frontend route does not establish create eligibility. Availability gates execution; an unavailable or disabled item is not a runnable offering.
3. **The default Marketplace presents only actionable and currently available offerings** alongside explicitly reviewed Remote MCP Integration recipes such as AnySearch (`Connect`). MCP, OpenAPI and Model Tools are *first-class visible built-in Provider offerings*. A selected Add must open exactly the owning resource's validated create flow with immutable plugin identity, not an all-plugin form or a new Provider for each integration recipe.
4. **Installed packages have a separate management view** (same page, focused switch) showing exact native installations including disabled, invalid or degraded records. Enable/disable/remove are installation-scoped actions governed by existing usage and safety checks. Built-ins are not fictitious installs. An installer action belongs here; Web still cannot pass native filesystem paths.
5. **Resource-owned Tool Suites and private Provider facades are not Marketplace products.** `agent.delegation`, `sandbox.workspace`, `sandbox.jobs` and `sandbox.browser` Tool Suite definitions remain reviewed Host contracts, and Workspace, Jobs and Agent Delegation Provider adapters retain their execution/ToolID identity; their preparation, adoption, repair, lifecycle and exposure belong only to the exact Agent/Sandbox detail workflow. No independent generic Provider chooser/Marketplace Add for those internal facades. Keep legacy unclaimed records visible for diagnosis without advertising a new manual creation path.
6. **Roadmap is not inventory.** E2B with no registered runnable backend and Browser Suite with no verified execution adapter do not appear in Marketplace. Docker/Daytona/Codex runtime unavailability is truthful; absence from *discoverable* offerings does not destroy the technical diagnostics or block a resource-owned setup/recovery flow.
7. **No new authorization.** Offering lookup, installation, creation, Suite binding, ToolID registration and Profile exposure remain separate. No implicit Profile binding, grants, Policy change or Agent loop. Hard-cut the outdated Marketplace projection; preserve IDs, pinned installations, metadata, persisted resources and stable tool source keys. Do not add compatibility readers, alternative legacy catalogues or a generic extension/service locator.

## Acceptance rules

- MCP, OpenAPI and Model Tools are directly visible and create correctly through focused Provider forms without an advanced toggle.
- Reviewed AnySearch is a Connect recipe using MCP; no second native executable or implied Credential/Profile authorization.
- No Workspace/Agent Delegation/Jobs/Browser Tool Suite cards, independent internal-facade Add, or E2B roadmap card in discovery; resource-detail Tool Suite controls remain.
- Supported installed extensions may be created when healthy; every exact installation is manageable/diagnosable after disable, corruption or degradation, without a forced create action. Unsupported `agent.backend` native packages do not masquerade as supported plugins.
- HTTP, SDK, Wails and Desktop convey identical host-owned action classification; UI never grants eligibility from capabilities or extension type alone.
- Tests cover fail-closed unknown built-ins/kinds, unavailable states, deep links, install management, and managed resource safety. No real credentials, users' runtime resources or remote processes touched by tests.

## Superseded presentation rule

ADR-0039 allowed `Bind (Tool Suite)` as a Marketplace action when ownership was still being explored. Tool Suite binding is now correctly resource-owned; this ADR retires *that presentation option only*, without removing Tool Suite definitions, capability negotiation, managed Provider facades or any execution/governance contract.
