# Architecture Decision Records

Architecture Decision Records (ADRs) capture decisions that materially constrain Mender's architecture or engineering model.

## Status values

- Proposed: under discussion; implementation must not assume acceptance.
- Accepted: active architectural constraint.
- Superseded: replaced by a newer ADR.
- Deprecated: retained for history but should not guide new implementation.
- Rejected: considered and intentionally not adopted.

## Process

1. Create a new ADR from `template.md`.
2. Use the next sequential number.
3. Describe context and competing forces before the decision.
4. Record meaningful alternatives, not straw-man options.
5. Identify consequences, including costs and limitations.
6. Merge the ADR before or with the first implementation that depends on it.
7. Never rewrite an accepted ADR to hide history. Supersede it with a new ADR when the decision changes.

Minor implementation choices that remain inside an accepted architectural boundary do not require an ADR.

## Index

| ADR | Decision | Status |
| --- | --- | --- |
| [0001](0001-monorepo.md) | Use a monorepo | Accepted |
| [0002](0002-ddd-clean-architecture.md) | Organize backend around DDD and Clean Architecture | Accepted |
| [0003](0003-canonical-tool-model.md) | Use a protocol-independent canonical tool model | Accepted |
| [0004](0004-runtime-modes-and-mcp.md) | Support local and remote modes with MCP as an adapter | Accepted |
| [0005](0005-frontend-desktop-stack.md) | Standardize the frontend stack and keep Wails as a shell | Accepted |
- [ADR-0006: Use database/sql with a pure-Go SQLite driver for local persistence](0006-sqlite-persistence.md)
- [ADR-0007: Centralize execution with data-minimized audit and execution-boundary credentials](0007-invocation-security-audit.md)
- [ADR-0008: Treat MCP as an anti-corruption layer and serve 2026-07-28 stateless HTTP](0008-mcp-adapter.md)
- [ADR-0009: Make the CLI a first-class local adapter with stable JSON contracts](0009-cli-contract.md)
- [ADR-0010: Use an OpenAPI-first loopback management control plane](0010-management-api.md)
- [ADR-0011: Isolate Wails as a desktop shell with a host-neutral management client](0011-wails-desktop-host.md)
- [ADR-0012: Harden local management and upstream Provider execution boundaries](0012-local-security-hardening.md)
- [ADR-0013: Require verifiable release artifacts and supply-chain evidence](0013-release-supply-chain.md)
- [ADR-0014: Fail closed on future SQLite schemas and use forward-only migrations](0014-sqlite-upgrade-compatibility.md)
- [ADR-0015: Persist HTTP header templates and resolve secrets at the Provider connection boundary](0015-http-provider-header-templates.md)
- [ADR-0016: Manage downstream MCP HTTP servers as ephemeral Runtime-owned instances](0016-managed-downstream-mcp-servers.md)
- [ADR-0017: Govern invocation with explicit execution scope and preflight/postflight stages](0017-governed-invocation-pipeline.md)
- [ADR-0018: Represent expected invocation rejections and failures as canonical outcomes](0018-canonical-invocation-outcomes.md)
- [ADR-0019: Make Profile ToolBinding the per-Profile governance unit](0019-profile-tool-binding-governance.md)
- [ADR-0020: Separate Credential inventory metadata from secret material](0020-credential-inventory-secret-backends.md)
- [ADR-0021: Bind Provider authentication to Credential References and enforce Profile credential scope](0021-provider-credential-bindings.md)
- [ADR-0022: Persist an unordered PolicySet with deterministic most-restrictive evaluation](0022-persistent-policy-set.md)
- [ADR-0023: Bound Policy matching to stable execution metadata and reviewed Governance Facts](0023-bounded-policy-match-language.md)
- [ADR-0024: Make Provider plugins the runtime boundary and treat MCP transport as plugin configuration](0024-provider-plugin-runtime.md)
- [ADR-0025: Map OpenAPI operations into governed Tools through a bounded Provider plugin](0025-openapi-provider-plugin.md)
- [ADR-0026: Bridge provider-hosted model tools through a normalized agentic Provider](0026-model-tool-bridge-provider.md)
- [ADR-0027: Host installable Provider plugins out of process behind a versioned Mender protocol](0027-external-provider-plugin-host.md)
- [ADR-0028: Make Provider plugin resilience and governance host-owned](0028-provider-plugin-resilience.md)
- [ADR-0029: Manage Agent Skills as versioned resources and keep deployment host-specific](0029-skill-resource-management.md)
- [ADR-0030: Provide governed local workspace capabilities without becoming an Agent harness](0030-workspace-native-tools-provider.md)
- [ADR-0031: Model external Profile reachability as managed Tunnel adapters](0031-managed-profile-tunnels.md)
- [ADR-0032: Govern Provider activation with explicit Principals and Policy actions](0032-provider-activation-principal-governance.md)
- [ADR-0033: Bound Runtime resource use and isolate local process execution](0033-runtime-budget-restricted-host-isolation.md)
- [ADR-0034: Govern every Runtime-owned native process through one host-owned process boundary](0034-runtime-owned-process-governance.md)
- [ADR-0035: Separate governed Sandbox resources from Provider execution routing](0035-governed-sandbox-execution-router.md) — Accepted
- [ADR-0036: Explicit Profile Service Principal Delegation to Sandboxes](0036-sandbox-profile-delegation.md) — Accepted
- [ADR-0037: Multi-kind Mender Plugin Platform](0037-multi-kind-plugin-platform.md) — Accepted
- [ADR-0038: Governed Agent Delegation](0038-governed-agent-delegation.md) — Accepted
- [ADR-0039: Marketplace Offerings, Resources and Tool Suites](0039-offerings-resource-tool-suites.md) — Accepted
- [ADR-0040: Versioned optional capability contracts without a Mender job runtime](0040-optional-plugin-capability-contracts.md) — Accepted
- [ADR-0041: SSH Jobs lifecycle invariants and evidence-bound recovery](0041-ssh-jobs-lifecycle-recovery.md) — Accepted
- [ADR-0042: P2 durable resource contracts and native plugin isolation posture](0042-p2-contracts-isolation-complexity.md) — Accepted
- [ADR-0043: Hard retire dual MCP config persistence and Workspace v1 adapters](0043-hard-retire-provider-compatibility.md) — Accepted
