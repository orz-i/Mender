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

## 7. One governed execution pipeline

All tool execution paths must converge on the same application pipeline and carry an explicit non-anonymous Principal:

```text
Resolve Execution Scope
   -> Resolve Tool
   -> Validate Arguments
   -> Derive Governance Facts
   -> Evaluate Policy / Approval
   -> Apply Execution Guard
   -> Execute
   -> Apply Result Guard
   -> Record Audit
   -> Return Invocation Outcome
```

CLI, MCP, Skill, Web, and Desktop clients must not implement independent execution semantics. Expected Tool/rejection failures are canonical outcomes; internal runtime failures remain distinguishable from those outcomes.

Global Policy must be deterministic and adapter-independent. Persistent rules explicitly select governance actions and have no priority/order semantics: the default decision applies only when no enabled rule matches, and matching rules combine by most-restrictive decision. Policy consumes stable Principal/origin/scope/target metadata and bounded reviewed Governance Facts rather than raw Tool arguments or secret-bearing context.

Provider capability acquisition is also governed. Production paths must admit provider.activate before Plugin.Open, external process launch, discovery Credential resolution, upstream Tool discovery, or executor-route registration. A denied Tool invocation is not sufficient proof if Provider activation already happened outside Policy.

## 8. Credentials are references until the execution boundary

Domain and application objects refer to Credentials by stable Reference. Inventory metadata may be persisted, but secret bytes must live behind a dedicated secret backend.

Secret material is resolved only at infrastructure execution/connection boundaries and must not be inserted into Tool descriptions, Tool schemas, model prompts, domain events, Provider persistence, Profile persistence, or normal Audit payloads. Native keyring failure must not fall back to plaintext storage.

## 9. Discovery is first-class

Mender must support large catalogs without requiring clients to load every tool schema.

The core information flow should support:

```text
Search -> Describe -> Invoke
```

Direct enumeration remains available for small profiles and protocol compatibility.

## 10. Profiles are operational and security boundaries

A Profile is more than a list of Tools. Its Profile-to-Tool relationship is a ToolBinding that may define exposure, machine/model presentation, restrictive policy, execution constraints, and an inventory-validated Credential allowlist.

Remote endpoints may bind directly to Profiles. Profile-scoped ToolBinding membership must be enforced by the shared Invocation pipeline; adapter checks may narrow discovery/projection but are not the authoritative execution boundary. Binding policy may tighten global Policy but must never relax it.

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

Provider trust, Principal attribution, capability admission, namespace collision, credential scope, execution limits, policy evaluation, and auditability are core concerns.

Security enforcement must not rely solely on metadata supplied by upstream tools or unauthenticated descriptive caller strings. Adapters may only construct Principals according to an explicit assurance boundary; they must not invent stronger remote identity than the transport actually proves.
