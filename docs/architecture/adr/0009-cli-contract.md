# ADR-0009: Make the CLI a first-class local adapter with stable JSON contracts

- Status: Accepted
- Date: 2026-09-26

## Context

Mender's product direction is local-first. By the end of Phase 4 the core runtime can synchronize and invoke MCP Providers and expose Profile-scoped MCP servers, but a user still cannot configure or operate those capabilities from the command line.

Agent Skills also need a stable local interface. Depending on human-formatted terminal text would make skills fragile and force Tool schemas into instructions.

Provider transport configuration introduces another boundary problem. Catalog Provider is protocol-independent, so endpoint/executable configuration must not be added to the Catalog Domain merely to support CLI persistence.

## Decision

Mender's CLI is a first-class adapter that calls application/runtime boundaries directly.

It does not require or call a local HTTP management API.

The CLI uses Cobra v1.10.2.

Commands return human-readable text by default and support a stable `--output json` mode. Agent Skills and automation must use JSON mode.

Persistent Profile state becomes a proper Profile application/infrastructure implementation.

MCP Provider connection configuration is persisted by MCP infrastructure in its own SQLite table keyed by canonical ProviderID. It does not modify Catalog Provider Domain.

Persisted MCP stdio config contains executable, argv, and working directory but not environment variable values. Persisted remote config contains endpoint but no tokens/passwords.

The Agent Skill remains thin and performs progressive disclosure through `tool search`, `tool describe`, and `tool call`.

## Alternatives considered

### Build the Web management API before the CLI

This would make local mode depend on running an HTTP service and contradict the local-first CLI requirement.

### Use human output as the Skill interface

This is convenient initially but brittle for agents and scripts.

### Persist MCP endpoint/process config directly on Catalog Provider

This couples the canonical Provider model to MCP and makes future OpenAPI/WASM/provider types harder to introduce.

### Persist stdio environment variables

Environment entries often contain credentials. Persisting them before Credential configuration exists would create an avoidable secret-storage path.

### Generate one Skill per Tool

This duplicates Catalog schemas into filesystem artifacts and defeats dynamic discovery for large catalogs.

## Consequences

### Positive

- Local workflows work without Web/Desktop.
- Agent Skills use a compact machine-readable contract.
- Catalog remains protocol-independent.
- Provider config persistence has a clear adapter owner.
- Large catalogs retain progressive disclosure.
- Future Web/Desktop can reuse the same application boundaries.

### Negative / trade-offs

- CLI and future management API require separate delivery adapters.
- Stable JSON output becomes a compatibility commitment.
- Provider registration spans Catalog plus adapter config persistence and requires compensating behavior on partial failure.
- Secret-bearing environment configuration remains unavailable until Credential management is designed.

## Follow-up

Phase 5 implements persistent Profile/application support, MCP Provider config storage, the Cobra command tree, and the repository Agent Skill.

Phase 6 builds management APIs and Web UI over the same application boundaries rather than wrapping CLI commands.
