# Phase 7 Plan — Wails Desktop Distribution

Status: Complete

## Objective

Package Mender's existing management application as a native desktop application without introducing a second product implementation or a second runtime path.

Phase 7 delivers:

- a Wails v3 desktop shell pinned to a reviewed prerelease,
- direct ownership of the existing Mender Runtime by the desktop process,
- a Wails binding adapter over the same Runtime/Application semantics used by CLI and server adapters,
- a transport-neutral frontend `MenderClient` contract shared by browser HTTP and desktop Wails hosts,
- the existing `packages/app` React product UI mounted unchanged by the desktop host,
- embedded Vite production assets,
- native build/package scaffolding for macOS, Windows, and Linux,
- desktop-specific tests and architecture boundary checks.

Phase 7 does not add new policy, credential, provider, or invocation semantics.

## Research baseline

Research date: 2026-09-26.

Wails v3 remains prerelease software. The newest published release reviewed for this phase is `v3.0.0-beta.26` (released 2026-09-25). Its release notes still mark v3 as Beta while stating the API is stable.

Mender pins both:

- Go module: `github.com/wailsapp/wails/v3 v3.0.0-beta.26`
- frontend runtime: `@wailsio/runtime 3.0.0-beta.26`

Do not use floating `latest` in committed project dependencies.

Primary references:

- https://github.com/wailsapp/wails/releases/tag/v3.0.0-beta.26
- https://v3.wails.io/concepts/build-system/
- https://v3.wails.io/concepts/lifecycle/
- https://v3.wails.io/features/bindings/services/
- https://v3.wails.io/features/bindings/models/
- https://v3.wails.io/reference/cli/
- https://v3.wails.io/guides/build/building/
- https://v3.wails.io/guides/build/cross-platform/
- https://v3.wails.io/guides/build/macos/
- https://v3.wails.io/guides/build/windows/
- https://v3.wails.io/guides/build/linux/

Wails v3 uses Task as the default project build orchestration. `wails3 build` delegates to the project's `build` task. Production assets are embedded with Go `embed`. Bindings may be generated to an explicit directory with `wails3 generate bindings -d ... -ts -i`.

## Desktop boundaries

The desktop distribution is an adapter composition:

```text
packages/app
    |
MenderClient
   / \
HTTP  Wails
 |      |
menderd desktop binding adapter
   \    /
   Runtime/Application
        |
      Domain
```

Rules:

1. `packages/app` contains no Wails imports and no browser fetch calls.
2. `packages/sdk` owns the product-facing `MenderClient` TypeScript contract and the browser HTTP implementation.
3. `apps/desktop/frontend` owns the Wails implementation of `MenderClient`.
4. Generated Wails bindings are host-local and must not enter `packages/app`.
5. Wails Go packages are imported only by the desktop shell module.
6. Desktop binding methods delegate into the existing `bootstrap.Runtime` and existing application use cases.
7. No desktop adapter calls an executor directly.
8. CLI and daemon import graphs must not contain Wails packages.

## Go module isolation

The Wails shell is a separate Go module at `apps/desktop`.

Its module path remains under the backend internal visibility prefix so it may consume existing composition/application adapters without exporting backend internals:

`github.com/orz-i/Mender/backend/desktop`

The root `go.work` includes:

- `./backend`
- `./apps/desktop`

The desktop module depends on the backend module and Wails. Because the backend module is not published as an independent versioned module, `apps/desktop/go.mod` uses a repository-local `replace github.com/orz-i/Mender/backend => ../../backend`. The backend module does not depend on Wails.

This is deliberate dependency isolation: adding desktop distribution must not make `mender` or `menderd` require native WebView dependencies or Wails SDK packages.

## Runtime ownership and lifecycle

The desktop process owns exactly one local `bootstrap.Runtime`.

Startup:

1. Resolve the desktop database path.
2. Create the application-data directory when needed.
3. Construct `bootstrap.NewRuntime`.
4. Construct the desktop binding service over that Runtime.
5. Construct the Wails application and register only the binding service required by the product UI.
6. Create the main WebView window.
7. Enter `app.Run()`.

Shutdown:

- Wails `OnShutdown` closes the Runtime.
- a guarded deferred close remains as a fallback if startup/run exits unusually.
- Runtime close is idempotently guarded at the shell boundary.

The Wails service must not own a second Runtime and must not start `menderd` internally.

## Desktop database path

Desktop defaults to an OS application-data/config location derived from `os.UserConfigDir()`:

`<UserConfigDir>/Mender/mender.db`

This maps to the platform's normal per-user application configuration area and avoids storing mutable state inside the installed application bundle.

For development/test control, `MENDER_DB` remains an explicit override.

The desktop shell creates the parent directory with user-only permissions where supported.

The browser/daemon database-path rules remain unchanged.

## Binding surface

The desktop binding adapter mirrors the management capabilities already exposed to the Web UI:

- system status,
- Provider list/create/sync/delete,
- Tool search/describe/call,
- Profile list/create/get/add/remove,
- Audit list,
- Policy status,
- Credential status.

Policy and Credential remain read-only.

Provider stdio configuration does not gain environment/secret fields.

Desktop Tool invocation uses caller `desktop` so Audit can distinguish it from `web`, `cli`, and MCP callers.

Binding DTO fields use explicit JSON tags matching the existing management/OpenAPI field names. Wails binding generation respects those JSON names in generated TypeScript.

## Frontend client contract

`packages/sdk` changes `MenderClient` from an inferred HTTP-client return type into an explicit interface.

The existing `createMenderClient` HTTP factory implements that interface.

The desktop host implements the same interface by adapting generated Wails bindings.

This keeps all product pages transport-neutral:

```text
packages/app -> MenderClient
apps/web -> createMenderClient() -> HTTP
apps/desktop/frontend -> createDesktopMenderClient() -> Wails bindings
```

The OpenAPI-generated schemas remain the canonical management DTO types used by the frontend contract. Wails bindings are checked structurally at the desktop adapter boundary.

Only `packages/sdk` may call `fetch`.

## React Router strategy

Browser host:

- `BrowserRouter`

Desktop host:

- `HashRouter`

The desktop WebView loads embedded assets from Wails' application asset server. Hash routing keeps route state after `#` and does not require the asset server to resolve arbitrary history paths back to `index.html`.

`packages/app` continues to define only route content; host selection of router implementation remains outside the product UI.

## Asset and Vite integration

The desktop frontend is `apps/desktop/frontend`, a pnpm workspace package.

It imports:

- `@mender/app`,
- `@mender/sdk`,
- `@mender/ui`,
- React 19,
- React Router,
- Wails runtime.

Vite production output is `apps/desktop/frontend/dist`.

The Wails shell embeds:

```go
//go:embed all:frontend/dist
```

and serves it with Wails application assets.

The desktop Vite dev server uses the Wails-provided dev port and Wails Vite plugin. Production does not require an HTTP management server.

## Wails build model

Use the Wails v3 generated Task/build-assets structure as the native build baseline rather than inventing packaging scripts.

Required local commands:

- `wails3 dev`
- `wails3 build`
- `wails3 package`
- `wails3 generate bindings -ts -i`

Wails-generated platform build assets are committed where they are source/configuration. Generated binaries, frontend `dist`, and local build outputs are ignored.

Bindings are generated source and are committed so normal TypeScript checks do not require a globally installed Wails CLI. A repository check regenerates them using the pinned Wails CLI version and verifies no diff.

## Packaging matrix

Phase 7 establishes build/package configuration for:

| Platform | Architecture | Native package |
| --- | --- | --- |
| macOS | arm64, amd64; universal when required | `.app`; DMG task available |
| Windows | amd64, arm64 | NSIS by default; MSIX available |
| Linux | amd64, arm64 | AppImage / deb / rpm / Arch tasks |

Phase 7 validates the native build on the current macOS host and checks configuration for all three platforms.

Cross-platform production release signing/notarization policy is Phase 8. Phase 7 does not claim unsigned cross-built artifacts are production-ready.

Linux WebKit/GTK compatibility follows Wails v3 defaults. Legacy GTK opt-in is not enabled unless an actual supported target requires it.

## Native integrations

Phase 7 intentionally does not add:

- file pickers,
- arbitrary URL opening,
- system tray,
- notifications,
- deep links,
- auto-start,
- custom protocol handlers.

None is required for the current management workflows. They may be introduced later only when backed by a concrete product use case and security review.

## Security boundaries

Desktop is local-first but not privileged around Runtime rules.

All Tool calls still execute through:

`Policy -> Approval -> Credential -> Executor -> Audit`

The desktop binding adapter cannot bypass policy or credential resolution.

No secret input/storage UI is introduced.

The desktop app does not relax `menderd`'s loopback-only management restriction.

Bindings must not expose raw database handles, executors, credential resolvers, or MCP connection objects.

## Testing

### Host-neutral client

- HTTP client still satisfies `MenderClient`.
- Desktop client satisfies `MenderClient`.
- product UI tests continue to use the same contract.

### Desktop binding adapter

Use a temporary SQLite database and existing test MCP providers/fakes where appropriate.

Cover:

- status,
- Provider lifecycle,
- Catalog search/describe,
- Profile membership,
- Tool invocation through the shared pipeline,
- Audit caller `desktop`,
- Policy/Credential read-only status.

### Lifecycle/path

Cover:

- default app-data path construction,
- `MENDER_DB` override,
- parent directory creation,
- runtime close guard.

### Architecture checks

Verify:

- no Wails import under backend Domain/Application packages,
- no Wails import in `backend/cmd/mender` or `backend/cmd/menderd`,
- no Wails import in `packages/app` or `packages/sdk`,
- no direct `fetch` outside `packages/sdk`,
- desktop frontend imports shared `@mender/app` rather than duplicating pages.

### Native smoke

On the current development host:

- generate bindings,
- build desktop frontend,
- run Wails native build,
- inspect that embedded application starts against a temporary/development DB where practical.

## Commit segmentation

1. `docs: define Wails desktop architecture`
2. `build(desktop): scaffold Wails v3 host`
3. `feat(web): add host-neutral management client`
4. `feat(desktop): add runtime bindings`
5. `build(desktop): wire shared React application`
6. `test(desktop): verify desktop host boundaries`
7. `docs: complete Wails desktop phase`

Small corrective commits are acceptable when a gate exposes an issue. Failed states are not committed.

## Closeout

Phase 7 completed on 2026-09-26.

Validated on the current macOS development host:

- `wails3 build` completed with the shared React application embedded in the native binary.
- `wails3 package` produced an ad-hoc-signed `mender-desktop.app`.
- the checked-in Wails build assets contain macOS, Windows, and Linux build/package tasks.
- `pnpm check` passed with binding regeneration, desktop architecture checks, Go lint/tests/race, ESLint, strict TypeScript, 4 Vitest files / 11 tests, and both Web/Desktop production frontend builds.

The current macOS linker emits a warning that one native object was built for macOS 13 while the Wails task targets macOS 12. The resulting native build and `.app` package succeed. Production deployment-target policy, release signing, and notarization remain Phase 8 work.

## Exit checklist

- [x] Wails v3 version is pinned from a documented research baseline.
- [x] Desktop Wails dependency is isolated from the backend CLI/server module.
- [x] Desktop owns and closes one shared Mender Runtime.
- [x] Desktop DB defaults to a per-user application-data/config directory.
- [x] A Wails binding adapter exposes existing management semantics only.
- [x] Desktop invocation still enters the shared invocation pipeline.
- [x] `MenderClient` is an explicit transport-neutral frontend contract.
- [x] Browser HTTP client implements `MenderClient`.
- [x] Desktop Wails client implements `MenderClient`.
- [x] `packages/app` has no Wails-specific imports.
- [x] Desktop mounts the existing `packages/app` UI without copied pages.
- [x] Desktop uses HashRouter while Web keeps BrowserRouter.
- [x] Vite production assets are embedded by the Wails shell.
- [x] Wails bindings are generated reproducibly from the pinned tool version.
- [x] Policy and Credential UI remain read-only.
- [x] No unnecessary native integration is added.
- [x] macOS native build succeeds locally.
- [x] Windows/Linux packaging configuration is present.
- [x] CLI/server dependency graphs contain no Wails packages.
- [x] Go tests/lint, strict TypeScript, Vitest, Vite builds, and architecture checks pass.
- [x] Full `pnpm check` passes.
