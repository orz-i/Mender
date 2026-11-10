# Local Development

## Prerequisites

Mender's foundation currently expects:

- Go 1.27 or newer.
- Node.js 22 or newer; Node.js 24 is the CI baseline.
- pnpm 12.4.2.
- golangci-lint v2.14.0.
- A Go-supported C toolchain with CGO enabled for race checks.

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

1. Portable script tests and backend architecture boundary verification.
2. Plugin protocol lint/regeneration/parity and SDK/example lint, tests, race checks and builds.
3. Isolated Go module verification.
4. OpenAPI TypeScript and Wails binding regeneration/diff verification.
5. Desktop architecture boundary verification.
6. Backend golangci-lint, tests and race tests.
7. ESLint, strict TypeScript type checking, Vitest and i18n coverage.
8. Web and Desktop production frontend builds.

The same command runs in Windows PowerShell/cmd and Unix shells. Plugin checks
use Node's argv, working-directory and environment APIs, including `GOWORK=off`;
example builds use disposable output directories rather than shell cleanup.
Repository `.gitattributes` keeps text in LF regardless of global
`core.autocrlf`, with explicit CRLF exceptions for native `.bat`/`.cmd` files.

Fixtures that require a verified restricted-host backend or a local POSIX
filesystem/shell report explicit capability skips when unavailable. Their
rejection paths remain tested. A local Windows SFTP fixture limitation does not
imply a Windows client cannot connect to a real Linux SSH server. See the
check portability record (historical) for the
failure classification and verification evidence.

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
pnpm scripts:test
pnpm backend:architecture:check
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

The repository now includes Catalog, the shared Invocation/Policy/Credential/Audit runtime, persistent Profiles, the generic Provider Plugin Runtime with directly configurable MCP, OpenAPI and Model Tools Providers plus resource-owned Sandbox Workspace Tools, downstream MCP adapters, the local CLI/Agent Skill workflow, the OpenAPI management control plane, typed TypeScript SDK, and shared React management application.

- `backend/` contains Go executable hosts, bounded contexts and adapters.
  `backend/internal/bootstrap` wires dependencies and owns Runtime shutdown.
  Shared management read models live in `management/application/query`.
- `apps/web/` is the browser host.
- `apps/desktop/` is the isolated Wails v3 desktop host and separate Go module.
- `packages/app/` contains the shared React shell and product features. Use
  the public Tools entry for services, Catalog and Workspace targets. Shared
  plugin configuration controls live under `src/shared/plugin-configuration`.
- `packages/ui/` provides product-independent UI primitives.
- `packages/sdk/` owns the management and local owner contracts and the
  browser HTTP adapter. OpenAPI output lives under `src/generated`.
- `contracts/` owns the OpenAPI management contract; `skills/` contains the Agent Skill surface.
- Wails remains isolated from `backend/go.mod`; CLI/server dependency graphs do not contain Wails.
- Both hosts supply a `ManagementClient`. Desktop additionally supplies
  optional local approval and Agent Tool placement capabilities and
  host callbacks. Web uses HTTP and Desktop uses Wails.

See the [system diagrams](../architecture/context.md) for the dependency and
execution boundaries. The Go architecture gate checks forbidden import
edges directly. ESLint checks the common client, shared feature code, the Tools
public entry and UI dependency direction.

Native Wails build/package and live backend checks are separate from the
portable gate. The real-environment acceptance matrix (historical)
lists prerequisites and the evidence to record.

## Quality policy

Do not bypass a failing repository gate by weakening compiler or linter settings without a reviewed architectural or engineering reason.

In particular:

- TypeScript strictness is part of the architecture baseline.
- golangci-lint configuration is repository-owned.
- `vite build` does not replace `pnpm typecheck`.
- Protocol or persistence dependencies must remain outside future domain packages.
