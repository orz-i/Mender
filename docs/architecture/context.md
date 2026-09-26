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

A named view over the catalog that controls exposure and execution constraints.

Profiles can be used by CLI sessions, MCP endpoints, Agent Skills, or management clients.

### Invocation

A request to execute a tool within a specific identity/profile/policy context.

An invocation may complete immediately, require additional input, fail, be cancelled, or become a long-running task.

### Policy

Rules that determine whether and how an invocation may proceed.

Policy may account for profile, identity, tool, provider trust, declared effects, filesystem/network scope, and approval requirements.

### Credential

A reference to authentication material required by a provider or tool.

Credential metadata can participate in domain decisions. Secret values remain infrastructure concerns.

### Audit Record

A durable record of an invocation lifecycle and relevant policy decisions, without unnecessarily persisting secret material.

## Adapter boundaries

### MCP inbound adapter

Converts MCP requests into application commands and queries.

It may expose a profile in direct, dynamic, or hybrid discovery mode.

### MCP provider adapter

Connects to upstream MCP servers, maps their tools into the canonical Catalog model, and delegates execution through the Invocation application layer.

### CLI adapter

Provides local search, describe, invoke, provider, profile, and server operations.

The CLI is a first-class interface, not a wrapper around HTTP.

### Agent Skill adapter

Provides progressive-disclosure instructions and optional generated resources that guide agents to use the Mender CLI/runtime.

### Management API

Provides administration operations for Web and Desktop clients. It is distinct from MCP protocol endpoints.

## Persistence

The initial local implementation is expected to use SQLite.

Persistence is accessed through context-specific ports. Database schema layout must not become the domain boundary.

A later remote deployment may use a different backing store without changing domain semantics.

## Composition

Dependency construction occurs at explicit composition roots used by the `mender` CLI, `menderd` daemon/server, and Wails desktop shell.

The same application use cases are reused by all entry points.
