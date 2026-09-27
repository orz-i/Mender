# System Context

## High-level view

```text
                          Humans / Agents
                                |
       +------------------------+------------------------+
       |                        |                        |
       v                        v                        v
      CLI                 MCP Clients              Agent Skills
       |                        |                        |
       +------------------------+------------------------+
                                |
                                v
                    Interfaces / Adapters
                                |
                                v
                      Application Layer
                                |
         +-----------+----------+----------+-----------+
         |           |                     |           |
         v           v                     v           v
      Catalog      Profile             Invocation    Policy
                                                \
                                                 v
                                      Credential / Audit
                                |
                                v
                           Domain Model
                                ^
                                |
              +-----------------+------------------+
              |                 |                  |
              v                 v                  v
         MCP Providers       Storage          Secret Stores
              |
              v
       External systems
```

## Core domain language

### Provider

A source of one or more tools.

Examples include a remote MCP server, a local MCP stdio process, a built-in Go provider, or a package/bundle adapted into a supported provider form.

Provider identity is stable and must not rely solely on upstream display names.

### Tool

A canonical executable capability known to Mender.

A Tool has stable identity, human/model-facing metadata, input and output schemas, provenance, and execution requirements.

A Tool is not an MCP-specific struct.

### Profile

A named view over the Catalog that controls exposure and execution constraints.

A Profile owns ToolBindings rather than copying canonical Tool state. A ToolBinding is the Profile-specific relationship to one Tool and can carry exposure/presentation overrides plus restrictive execution governance.

Profiles can be used by CLI sessions, MCP endpoints, Agent Skills, or management clients.

### ToolBinding

The Profile-owned relationship between one Profile and one canonical Tool.

ToolBinding may customize Profile-facing alias/title/description and may tighten execution policy or serialize calls for that binding. ToolID remains canonical identity; schemas and provenance remain Catalog-owned.

### Invocation

A governed request to execute a Tool within an explicit origin and execution scope.

The scope is either privileged Catalog scope or one Profile scope. Invocation owns Profile membership enforcement for Profile-scoped calls, validates input before Policy/execution, applies execution/result guards, and returns a canonical outcome.

An invocation may complete immediately, require additional input, be rejected, fail, be cancelled, or become a long-running task.

### Policy

A persistent Runtime-wide PolicySet that determines whether and how an Invocation may proceed.

Policy rules are unordered and evaluate bounded execution metadata plus reviewed Governance Facts. Matching deny decisions dominate require-approval/allow rules; the default decision applies only when no rule matches. Profile ToolBinding policy remains a second tightening layer. Raw arguments, secrets, Caller identity, and Credential References do not cross the Policy boundary in Phase 12.

### Credential

A human-manageable Reference plus non-secret metadata for authentication material required by a Provider or Tool.

Credential inventory metadata may be persisted and selected by Profile/Provider configuration. Secret values remain infrastructure concerns and are resolved only from native OS credential storage or environment-backed sources at execution/connection boundaries.

### Audit Record

A durable terminal record of an invocation lifecycle, execution scope/origin, outcome classification, and relevant policy decisions, without unnecessarily persisting secret material or Tool payloads.

## Adapter boundaries

### MCP inbound adapter

Converts MCP requests into application commands and queries.

It may expose a profile in direct, dynamic, or hybrid discovery mode.

### MCP provider adapter

Connects to upstream MCP servers, maps their tools into the canonical Catalog model, and delegates execution through the Invocation application layer. Remote non-loopback HTTP transport requires TLS; local stdio processes run without a shell and receive a reduced ambient environment. These controls reduce authority but do not claim kernel-level Provider sandboxing.

### CLI adapter

Provides local search, describe, invoke, provider, profile, and server operations.

The CLI is a first-class interface, not a wrapper around HTTP.

### Agent Skill adapter

Provides progressive-disclosure instructions and optional generated resources that guide agents to use the Mender CLI/runtime.

### Management HTTP API

Provides the loopback administration transport used by the browser host. It is distinct from MCP protocol endpoints.

### Desktop binding adapter

Adapts the same Runtime/Application management semantics to Wails bindings without routing through the management HTTP server. Wails SDK imports and WebView lifecycle remain in the desktop shell; the backend adapter itself is Wails-independent.

## Persistence

The initial local implementation is expected to use SQLite.

Persistence is accessed through context-specific ports. Database schema layout must not become the domain boundary.

A later remote deployment may use a different backing store without changing domain semantics.

## Composition

Dependency construction occurs at explicit composition roots used by the `mender` CLI, `menderd` daemon/server, and Wails desktop shell.

The same application use cases are reused by all entry points.
