# Architecture Principles

These principles are repository-level constraints. Changes that conflict with them require an ADR.

## 1. Tool is a domain concept

Mender's internal model must not be defined by MCP SDK types, CLI flags, React state, database rows, or Wails bindings.

MCP, CLI, Agent Skills, HTTP APIs, and desktop bindings are adapters around the application and domain layers.

## 2. Dependencies point inward

The intended dependency direction is:

```text
Interfaces / Infrastructure
            |
            v
       Application
            |
            v
          Domain
```

The domain layer must not depend on transport protocols, MCP SDKs, databases, filesystems, environment variables, web frameworks, UI frameworks, or Wails.

Application code may depend on domain abstractions and ports defined by the consuming use case. Infrastructure implements those ports.

## 3. Interfaces belong to consumers

Go interfaces should normally be declared near the application code that consumes them.

Avoid global interface packages such as `interfaces`, `repositories`, or `contracts` that accumulate unrelated abstractions.

## 4. Organize by bounded context

Backend code is organized around business capabilities rather than horizontal technical layers.

Initial bounded contexts are expected to include Catalog, Profile, Invocation, Policy, Credential, and Audit.

Discovery begins as an application capability around Catalog and may become its own bounded context only when its domain behavior warrants that separation.

## 5. Cross-context access is explicit

A bounded context owns its behavior and data semantics.

One context must not reach into another context's repository or database tables as a shortcut. Cross-context collaboration happens through explicit application boundaries.

A shared physical database does not imply shared domain ownership.

## 6. Infrastructure types do not leak inward

Protocol and persistence models are mapped at boundaries.

For example:

```text
mcp.Tool -> mapper -> catalog domain Tool
```

Domain entities must not contain MCP SDK structs, SQL row types, HTTP request types, or Wails-generated types.

## 7. One invocation pipeline

All tool execution paths must converge on the same application pipeline:

```text
Resolve Tool
   -> Evaluate Policy
   -> Resolve Credentials
   -> Execute
   -> Record Audit
   -> Return Result
```

CLI, MCP, Skill, Web, and Desktop clients must not implement independent execution semantics.

## 8. Credentials are references until the execution boundary

Domain and application objects refer to credentials by identifier or reference.

Secret material is resolved only at the infrastructure execution boundary and must not be inserted into tool descriptions, tool schemas, model prompts, domain events, or normal audit payloads.

## 9. Discovery is first-class

Mender must support large catalogs without requiring clients to load every tool schema.

The core information flow should support:

```text
Search -> Describe -> Invoke
```

Direct enumeration remains available for small profiles and protocol compatibility.

## 10. Profiles are operational and security boundaries

A profile is more than a list of tools. It may define visible providers and tools, aliases, export behavior, policy, credential scope, and approval requirements.

Remote endpoints may bind directly to profiles.

## 11. Local-first, remote-native

The runtime must work without a hosted service or desktop UI.

At the same time, core APIs and storage abstractions must permit remote deployment without embedding local-only assumptions in domain code.

## 12. Wails is a shell

Wails v3 may provide desktop packaging, native integration, and transport to the shared application core.

The core must remain buildable and testable without Wails.

## 13. Strict static quality gates

Go code must pass the repository-pinned golangci-lint configuration and tests.

TypeScript must run with strict compiler settings, including additional checks such as unchecked indexed access and exact optional property types unless an ADR explicitly changes this policy.

## 14. Prefer explicit architecture over framework magic

Mender should minimize hidden dependency injection, runtime reflection for core domain wiring, and framework-specific conventions that obscure execution paths.

Composition roots should wire dependencies explicitly.

## 15. Security is a core runtime responsibility

Provider trust, namespace collision, credential scope, execution limits, policy evaluation, and auditability are core concerns.

Security enforcement must not rely solely on metadata supplied by upstream tools.
