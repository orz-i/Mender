# Mender

Mender is a local-first tool runtime and gateway for LLMs and agents.

It provides one canonical tool model and execution pipeline, then exposes the same tool catalog through multiple adapters:

- MCP for remote and local protocol access.
- CLI for direct local use and automation.
- Agent Skills for progressive discovery and agent-facing workflows.
- Web and desktop management surfaces built on the same application core.

## Status

Phases 0–13 are complete. Mender now combines the governed Invocation pipeline with a Provider Plugin Runtime: canonical Provider/Tool identity stays protocol-independent, upstream MCP is one built-in plugin with stdio and Streamable HTTP transport variants, and active Provider instances are reused/reloaded/closed through an explicit lifecycle manager. Credential and Policy governance remain shared across CLI, MCP, Web, and Desktop.

## Product principles

1. Tool is the core domain concept; MCP, CLI, and Skill are adapters.
2. Local-first does not mean local-only. The same core must support remote operation.
3. Discovery is a first-class capability. Large catalogs must not require eager schema injection.
4. Every invocation uses an explicit execution scope and passes through validation, policy, guarded execution, result governance, credential-boundary handling, and audit.
5. Domain code must not depend on transport, storage, UI, or protocol SDKs.
6. Web, CLI, daemon, and desktop clients share the same application core.
7. Security boundaries are designed into the runtime, not added after feature development.

See:

- [Product vision](docs/product/vision.md)
- [Architecture principles](docs/architecture/principles.md)
- [System context](docs/architecture/context.md)
- [Architecture Decision Records](docs/architecture/adr/)
- [Roadmap](docs/planning/roadmap.md)
- [Local development](docs/development/getting-started.md)
- [CLI reference](docs/development/cli.md)
- [Management Web UI](docs/development/web.md)
- [Wails Desktop](docs/development/desktop.md)
- [Release and distribution](docs/development/release.md)
- [Upgrade and downgrade](docs/development/upgrades.md)
- [Catalog performance](docs/development/performance.md)

## Development

Bootstrap and verify the complete repository:

```bash
pnpm install --frozen-lockfile
pnpm check
```

See [Local development](docs/development/getting-started.md) for individual commands and prerequisites.

## Technology stack

### Backend

- Go
- Official MCP Go SDK
- golangci-lint
- SQLite for the initial local persistence implementation

### Frontend

- React 19
- TypeScript with strict mode
- React Router
- Tailwind CSS 4
- shadcn/ui
- Motion
- Vite
- pnpm

### Desktop

- Wails v3 as an optional shell around the same core application

## Repository model

Mender uses a monorepo. Go code follows DDD-oriented bounded contexts, Clean Architecture, and dependency inversion. TypeScript code follows domain-oriented feature boundaries and strict static checking.

Implementation scaffolding will follow the accepted ADRs rather than define architecture implicitly through directory layout.
