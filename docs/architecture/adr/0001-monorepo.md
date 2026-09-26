# ADR-0001: Use a monorepo

- Status: Accepted
- Date: 2026-09-25

## Context

Mender includes a Go runtime, CLI, daemon, React management UI, shared TypeScript packages, protocol contracts, Agent Skills, and an optional Wails desktop client.

These pieces evolve together and share domain language, release constraints, generated contracts, CI policy, and architecture documentation.

Splitting them into separate repositories at project inception would introduce version coordination and cross-repository changes before there is a demonstrated need for independent ownership.

## Decision

Mender will use a monorepo.

The repository will contain:

- Go backend/runtime applications and internal packages.
- Web and optional desktop applications.
- Shared TypeScript UI/application/SDK packages.
- API and schema contracts.
- Agent Skills.
- Documentation, ADRs, CI, and repository governance.

Go modules will be coordinated through `go.work` where multiple modules are justified.

JavaScript and TypeScript packages will use pnpm workspaces.

A higher-level JavaScript build orchestrator such as Nx or Turborepo will not be introduced initially. It may be adopted later if repository scale demonstrates a concrete need for dependency-aware task scheduling or remote caching.

## Alternatives considered

### Separate repositories

Separate repositories could isolate release cadence and access control, but would make atomic changes across runtime, contracts, UI, and documentation harder during the formative stage.

### Single Go module and single frontend directory without workspace structure

This is simpler initially, but makes package boundaries and future desktop/web reuse less explicit.

## Consequences

### Positive

- Architectural changes can be committed atomically.
- Shared contracts and generated clients remain synchronized.
- One CI policy governs the complete product.
- Documentation and implementation history stay together.
- Web and desktop code can reuse application packages without publishing internal packages.

### Negative / trade-offs

- CI must avoid rebuilding unrelated areas as the repository grows.
- Repository ownership rules may need refinement later.
- Release tooling must support multiple artifacts from one repository.

## Follow-up

The scaffold phase will establish the workspace files and directory boundaries described by accepted ADRs.
