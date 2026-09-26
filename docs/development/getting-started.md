# Local Development

## Prerequisites

Mender's foundation currently expects:

- Go 1.27 or newer.
- Node.js 22 or newer; Node.js 24 is the CI baseline.
- pnpm 12.4.2.
- golangci-lint v2.14.0.

The repository pins pnpm in the root `package.json` and golangci-lint in `.golangci-lint-version`. Native desktop development additionally uses Wails v3.0.0-beta.26; the portable `pnpm check` gate regenerates bindings with that pinned version through `go run`, so a globally installed Wails CLI is only required for `wails3 dev`, native build, and packaging.

Install the pinned Go linter when needed:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
```

## Bootstrap

From the repository root:

```bash
pnpm install --frozen-lockfile
pnpm check
```

`pnpm check` is the repository-level verification command. It currently runs:

1. OpenAPI TypeScript regeneration/diff verification.
2. Wails binding regeneration/diff verification.
3. Desktop architecture boundary verification.
4. golangci-lint.
5. Go tests.
6. Go race tests.
7. ESLint.
8. strict TypeScript type checking.
9. Vitest.
10. Web and Desktop production frontend builds.

## Common commands

See [Management Web UI](web.md) for the local API + Vite development workflow and [Desktop Development](desktop.md) for Wails setup, native build, and packaging.

Run the web development server:

```bash
pnpm dev:web
```

See [CLI reference](cli.md) for provider, profile, discovery, invocation, and MCP serving workflows.

Run the CLI:

```bash
pnpm run:cli
```

Run the loopback management API:

```bash
pnpm run:server
```

Run individual portable quality gates:

```bash
pnpm desktop:bindings:check
pnpm desktop:architecture:check
pnpm go:lint
pnpm go:test
pnpm go:test:race
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

Run network-requiring dependency vulnerability scans separately:

```bash
pnpm security:check
```

Reproduce the Catalog scale benchmark with:

```bash
pnpm go:bench:catalog
```

Release and upgrade procedures are documented in [Release and Distribution](release.md) and [Upgrade and Downgrade](upgrades.md).

## Workspace boundaries

The repository now includes Catalog, the shared Invocation/Policy/Credential/Audit runtime, persistent Profiles, MCP adapters, the local CLI/Agent Skill workflow, the OpenAPI management control plane, typed TypeScript SDK, and shared React management application.

- `backend/` contains Go executable hosts, domain/application contexts, MCP infrastructure adapters, and composition roots.
- `apps/web/` is the browser host.
- `apps/desktop/` is the isolated Wails v3 desktop host and separate Go module.
- `packages/app/` is the shared React application shell.
- `packages/ui/` is the shared UI boundary and shadcn/ui destination.
- `packages/sdk/` is the generated/typed management API client boundary and the only browser HTTP boundary.
- `contracts/` owns the OpenAPI management contract; `skills/` contains the Agent Skill surface.
- Wails remains isolated from `backend/go.mod`; CLI/server dependency graphs do not contain Wails.
- Web and Desktop both mount `packages/app` behind the same `MenderClient` contract, using HTTP and Wails transports respectively.

Native Wails build/package checks are documented separately because they require platform GUI and packaging toolchains. Release signing, SBOM/checksum generation, dependency scanning, and database compatibility rules are repository-owned hardening concerns rather than UI behavior.

## Quality policy

Do not bypass a failing repository gate by weakening compiler or linter settings without a reviewed architectural or engineering reason.

In particular:

- TypeScript strictness is part of the architecture baseline.
- golangci-lint configuration is repository-owned.
- `vite build` does not replace `pnpm typecheck`.
- Protocol or persistence dependencies must remain outside future domain packages.
