# Product Vision

## Problem

LLM and agent tooling is fragmented across remote MCP servers, local MCP processes, CLIs, scripts, API integrations, and agent-specific skill formats. Each integration path tends to duplicate discovery, authentication, execution, and policy logic.

Large tool catalogs create a second problem: exposing every tool schema to a model is expensive, noisy, and degrades tool selection.

Mender exists to provide a single runtime and management plane for those capabilities.

## Product statement

Mender is a local-first tool runtime and gateway for LLMs and agents.

A tool is registered once into Mender's canonical catalog and can then be discovered, authorized, invoked, audited, and exposed through different adapters without duplicating execution logic.

## Primary capabilities

Mender should eventually provide:

- Provider registration and synchronization.
- A canonical tool catalog independent from MCP or any other protocol.
- Search, describe, and invoke workflows for large catalogs.
- Profiles whose ToolBindings combine membership, exposure, aliases/presentation, and restrictive execution governance.
- Credential references and runtime-only secret resolution.
- Invocation policy and optional approval gates.
- Auditable execution records.
- MCP server exposure.
- Local CLI access.
- Agent Skill generation and/or consumption.
- Web and desktop management clients.

## Runtime modes

### Local mode

Local mode prioritizes direct execution and composability.

Primary interfaces:

- `mender` CLI.
- MCP over stdio where protocol compatibility is needed.
- Agent Skills that instruct an agent to discover and invoke tools through the CLI.

The local runtime must not require a web server.

### Remote mode

Remote mode exposes selected profiles through MCP over Streamable HTTP.

The remote server should be deployable as a stateless application wherever protocol semantics permit. Persistent state belongs behind explicit infrastructure ports.

## Product boundaries

Mender is not initially intended to be:

- An agent framework.
- A model orchestration framework.
- A new MCP registry or marketplace.
- A proprietary replacement for MCP, Agent Skills, or MCPB.
- A general-purpose workflow engine.
- A hosted SaaS requirement for local operation.

These may become integrations or adjacent capabilities, but they must not distort the core runtime.

## Canonical workflow

The target execution flow is:

```text
Provider
   |
   v
Normalize
   |
   v
Catalog
   |
   +--> Search / Describe
   |
   v
Execution Scope (Catalog / Profile)
   |
   v
Input Validation
   |
   v
Governance Facts
   |
   v
Policy / Approval
   |
   v
Execution Guard
   |
   v
Credential-boundary Execution
   |
   v
Result Guard
   |
   +--> Audit terminal outcome
   |
   v
Invocation Outcome
```

Adapters enter this flow at application boundaries; they do not bypass it. Profile-scoped execution is enforced inside Invocation rather than delegated solely to protocol adapters. Profile ToolBinding policy can only tighten the global execution decision; presentation overrides remain outside Invocation.

## Primary users

Initial target users are:

- Developers using multiple MCP servers and command-line tools.
- Agent builders who need a single controlled tool surface.
- Teams that need repeatable tool profiles and credential boundaries.
- Power users who want local ownership of tool execution.

## Success criteria for the first usable release

A first usable release should allow a user to:

1. Register local and remote MCP providers.
2. Synchronize their tools into a canonical catalog.
3. Create a profile containing a subset of tools.
4. Search, inspect, and invoke tools from the CLI.
5. Expose the same profile over MCP.
6. Use the same runtime through an Agent Skill.
7. Review invocation history and policy decisions.
8. Keep secrets outside tool schemas, prompts, and audit payloads.
