# Local Development

## Prerequisites

Mender's foundation currently expects:

- Go 1.27 or newer.
- Node.js 22 or newer; Node.js 24 is the CI baseline.
- pnpm 12.4.2.
- golangci-lint v2.14.0.

The repository pins pnpm in the root `package.json` and golangci-lint in `.golangci-lint-version`.

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

1. golangci-lint.
2. Go tests.
3. Go race tests.
4. ESLint.
5. strict TypeScript type checking.
6. Vitest.
7. production frontend build.

## Common commands

Run the web development server:

```bash
pnpm dev:web
```

Run the current CLI foundation entry point:

```bash
pnpm run:cli
```

Run the current daemon foundation entry point:

```bash
pnpm run:server
```

Run individual quality gates:

```bash
pnpm go:lint
pnpm go:test
pnpm go:test:race
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

## Workspace boundaries

The repository now includes the Catalog bounded context; later product contexts remain intentionally absent until their owning phases begin.

- `backend/` contains Go executable hosts, the Catalog bounded context, and composition roots.
- `apps/web/` is the browser host.
- `packages/app/` is the shared React application shell.
- `packages/ui/` is the shared UI boundary and shadcn/ui destination.
- `packages/sdk/` reserves the typed management API client boundary.
- `contracts/` and `skills/` will be introduced when their owning phases begin.
- Wails is intentionally not required by the current CLI/server/web build.

The next implementation phase is Invocation, Policy, Credential, and Audit. It must reuse Catalog through application boundaries and must not bypass Catalog ownership through SQLite tables.

## Quality policy

Do not bypass a failing repository gate by weakening compiler or linter settings without a reviewed architectural or engineering reason.

In particular:

- TypeScript strictness is part of the architecture baseline.
- golangci-lint configuration is repository-owned.
- `vite build` does not replace `pnpm typecheck`.
- Protocol or persistence dependencies must remain outside future domain packages.
