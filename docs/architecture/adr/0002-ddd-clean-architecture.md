# ADR-0002: Organize backend around DDD and Clean Architecture

- Status: Accepted
- Date: 2026-09-25

## Context

Mender combines catalog management, profiles, execution, policy, credentials, and audit behavior. These concerns will gain independent rules over time.

A conventional horizontal structure such as `handlers`, `services`, `repositories`, and `models` makes technical layers visible but tends to hide domain ownership and encourages unrelated code to accumulate in shared packages.

Mender must also support multiple inbound and outbound adapters without allowing MCP, HTTP, SQLite, Wails, or CLI concerns to define the domain model.

## Decision

The Go backend will use domain-oriented bounded contexts, Clean Architecture dependency direction, and dependency inversion.

Initial bounded contexts are:

- Catalog.
- Profile.
- Invocation.
- Policy.
- Credential.
- Audit.

Within a bounded context, packages may separate domain, application, and infrastructure concerns where that separation is useful.

Dependency direction is inward:

```text
Interfaces / Infrastructure
            |
            v
       Application
            |
            v
          Domain
```

Interfaces are normally defined by their consumers. Infrastructure implements those ports.

Cross-context collaboration happens through explicit application APIs or ports. A context may not bypass another context by directly using its repository or database tables.

A shared physical database is allowed; logical domain ownership remains separate.

Composition roots explicitly assemble dependencies for the CLI, daemon, and desktop shell.

## Alternatives considered

### Traditional layered architecture

Horizontal folders are familiar but make bounded-context ownership weak and create pressure toward global service and repository abstractions.

### Framework-led architecture

A framework could dictate handlers, modules, dependency injection, and persistence patterns. This would reduce initial wiring but would couple core architecture to framework conventions.

### Pure hexagonal architecture without DDD boundaries

Ports and adapters solve dependency direction but do not by themselves define ownership between Catalog, Policy, Credential, and Invocation behavior.

## Consequences

### Positive

- Business language and ownership remain visible in the repository.
- Domain packages stay independent from infrastructure.
- Multiple delivery mechanisms can share the same use cases.
- Infrastructure can be replaced behind explicit ports.
- Context boundaries provide a basis for independent testing.

### Negative / trade-offs

- More deliberate mapping code is required at boundaries.
- Developers must resist unnecessary abstractions and "enterprise DDD" ceremony.
- Cross-context workflows require explicit orchestration rather than direct table access.

## Follow-up

Scaffolding and architecture tests should make forbidden dependency directions difficult to introduce accidentally.
