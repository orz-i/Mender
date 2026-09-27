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
