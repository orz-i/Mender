# ADR-0003: Use a protocol-independent canonical tool model

- Status: Accepted
- Date: 2026-09-25

## Context

Mender aggregates tools from different sources and exposes them through different interfaces.

If the internal model is built directly from MCP SDK types, MCP becomes the architecture rather than one protocol. That would make CLI, Agent Skill, future OpenAPI/WASM/A2A integrations, and protocol evolution harder to support cleanly.

Large catalogs also require search and describe operations that are application concerns rather than protocol-specific behavior.

## Decision

Mender will define its own canonical Provider, Tool, Profile, and Invocation models.

MCP and other integrations map to and from those models at infrastructure or interface boundaries.

A canonical Tool includes, at minimum:

- Stable Mender identity.
- Provider identity and provenance.
- Export/display names.
- Description and searchable metadata.
- Input schema.
- Output schema where available.
- Execution requirements.
- Security/policy-relevant metadata whose trust level is known.

JSON Schema is the canonical schema representation for tool inputs and outputs.

Tool identity is distinct from upstream names and from exported protocol names so collisions and renames can be managed safely.

The application layer must support the conceptual workflow:

```text
Search -> Describe -> Invoke
```

Invocation results must be extensible beyond immediate success/failure so multi-step input requirements and long-running tasks can be represented without redesigning the domain.

## Alternatives considered

### Use MCP SDK types as the internal model

This minimizes initial mapping code but creates protocol coupling throughout the domain and application layers.

### Define separate internal models for each adapter

This avoids coupling to one protocol but duplicates semantics and makes policy, audit, and execution behavior inconsistent.

## Consequences

### Positive

- MCP can evolve without forcing protocol types into the domain.
- CLI and Agent Skill adapters use the same semantics.
- Tool search and profile behavior are protocol-independent.
- New provider and exposure adapters can be added without replacing the core.

### Negative / trade-offs

- Mappers are required between MCP and canonical types.
- Some upstream protocol details may need explicit extension metadata.
- Canonical schema evolution becomes Mender's responsibility.

## Follow-up

The scaffold phase will define domain types without importing the MCP SDK into domain packages.
