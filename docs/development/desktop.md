# Desktop Development

Mender's desktop distribution is a Wails v3 shell around the same Runtime and shared React product application used by the other adapters.

## Version baseline

Phase 7 is pinned to Wails:

```text
v3.0.0-beta.26
```

Install that exact CLI version:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
wails3 version
```

The frontend runtime is also pinned to `@wailsio/runtime 3.0.0-beta.26`.

Wails v3 is still a prerelease dependency. Do not update the Go module, CLI, or frontend runtime independently. Review a new Wails release and regenerate bindings/build assets together.

## Architecture

The desktop host lives in:

```text
apps/desktop
```

It is a separate Go module. The root `go.work` composes it with `backend`, while `backend/go.mod` remains free of Wails dependencies.

The runtime path is:

```text
apps/desktop Wails shell
  -> backend/internal/desktop/binding
  -> bootstrap.Runtime / Application
  -> Domain
```

The frontend path is:

```text
apps/desktop/frontend
  -> Wails generated bindings
  -> desktop MenderClient adapter
  -> packages/app
```

The browser continues to use:

```text
apps/web
  -> packages/sdk HTTP client
  -> /api/v1
  -> menderd
```

`packages/app` does not import Wails bindings. It receives the same explicit `MenderClient` contract from either host.

The desktop process does not start `menderd` and does not use the management HTTP API internally.

## Database location

The desktop process resolves its default database under Go's per-user `os.UserConfigDir()`:

```text
<UserConfigDir>/Mender/mender.db
```

For development or isolated testing, override it with:

```bash
MENDER_DB=/absolute/path/to/mender.db wails3 dev
```

The parent directory is created before Runtime startup.

CLI and daemon database-path behavior is unchanged.

## Authenticated remote MCP Providers

HTTP Provider header templates are resolved from the **Mender desktop process environment** when a Provider connection is created. A typical template is:

```text
Authorization: Bearer ${ANYSEARCH_API_KEY}
```

Only the template is persisted. The resolved secret is not stored in the Mender SQLite database.

For local development, ensure the variable is present in the process that launches Mender, for example when using `wails3 dev`:

```bash
ANYSEARCH_API_KEY=... wails3 dev
```

On macOS, do not assume an environment variable that exists only in an interactive shell is available to an application launched separately through Finder/Launch Services. If a header template reports a missing variable, launch the development executable from an environment that contains it or configure the user session environment explicitly. Persistent desktop secret storage is not implemented yet.

## Frontend routing

The Web host uses `BrowserRouter`.

The desktop host uses `HashRouter` because the production UI is served from Wails' embedded asset server and must not require server-side history fallback for application routes.

Product routes remain defined only in `packages/app`.

## Generated bindings

Bindings are generated into:

```text
apps/desktop/frontend/bindings
```

Regenerate them from the desktop module:

```bash
cd apps/desktop
wails3 generate bindings -ts -i
```

The repository-level reproducibility check uses the pinned Wails CLI version:

```bash
pnpm desktop:bindings:check
```

Do not hand-edit generated bindings.

## Development

Install JavaScript dependencies from the repository root first:

```bash
pnpm install --frozen-lockfile
```

Then run the desktop development host:

```bash
cd apps/desktop
wails3 dev
```

Wails runs the Vite development server through the checked-in Task configuration. The frontend package manager is pnpm.

## Native build

Mender's macOS desktop baseline is macOS 13 Ventura or newer. The repository uses Go 1.27, whose Darwin port no longer supports macOS 12; the checked-in Wails deployment target and Info.plist minimum are therefore pinned to 13.0 even though the upstream Wails beta template currently defaults to 12.0.

Build for the current platform:

```bash
cd apps/desktop
wails3 build
```

Create the native package for the current platform:

```bash
wails3 package
```

The checked-in Wails v3 build assets provide:

- macOS: native arm64/amd64 builds, universal build task, `.app`, and DMG task.
- Windows: amd64/arm64 build path, NSIS default packaging, and MSIX support.
- Linux: amd64/arm64 build path plus AppImage, deb, rpm, and Arch packaging tasks.

Cross-platform native builds may require Docker and the platform prerequisites expected by Wails.

A native package is not automatically a production release. Public desktop artifacts require platform-native signing; macOS distribution additionally requires notarization. Generic CI must not publish an unsigned or merely ad-hoc-signed desktop package as a production release. See [Release and Distribution](release.md).

Phase 7 validated `wails3 build` and `wails3 package` on the current macOS development host. That local smoke package is ad-hoc-signed only.

## Quality gates

Run desktop-specific portable checks:

```bash
pnpm desktop:bindings:check
pnpm desktop:architecture:check
```

The architecture check verifies that:

- `backend/go.mod` does not depend on Wails,
- CLI and daemon dependency graphs do not contain Wails,
- `packages/app` and `packages/sdk` do not import Wails bindings,
- no frontend outside `packages/sdk` directly calls `fetch`,
- Web keeps `BrowserRouter`,
- Desktop keeps `HashRouter` and mounts `@mender/app`.

The full portable repository gate is still:

```bash
pnpm check
```

It includes binding regeneration, desktop architecture checks, desktop adapter Go tests, race tests, strict TypeScript, Vitest, and both Web/Desktop Vite production builds.

Native Wails build/package remain explicit platform smoke checks rather than part of `pnpm check`, because they require OS-specific GUI toolchains and packaging dependencies.

## Product scope

Desktop currently exposes the same management workflows as the Web UI:

- Providers,
- Catalog search/describe/invoke,
- Profiles,
- managed MCP Servers,
- Policy status,
- Credential resolver status,
- Audit.

Policy and Credential remain read-only. Desktop Tool calls use the same canonical Invocation outcomes as the management HTTP adapter, including `rejected` and `failed`; the Wails host adapter maps those outcomes without treating them as binding/transport failures. Audit details expose the same origin/scope/outcome metadata as Web.

The Desktop `Servers` page can start a Profile as a loopback Streamable HTTP MCP server without launching a second Mender process. Running servers are ephemeral Runtime state and stop with the Desktop process. The page provides endpoint/config copy plus Restart/Stop controls. stdio serving remains client-launched through the CLI because its lifetime is tied to the external client's stdin/stdout process.

Phase 7 intentionally does not add tray behavior, notifications, deep links, file dialogs, auto-start, or other native integrations without a product requirement.
