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

The frontend composes the shared application and both capability contracts.

```text
apps/desktop/frontend
  -> packages/app
       -> ManagementClient -> desktop adapter -> generated Wails bindings
       -> LocalOwnerCapabilities -> desktop owner adapter -> generated Wails bindings
```

The browser continues to use:

```text
apps/web
  -> packages/sdk HTTP client
  -> /api/v1
  -> menderd
```

`packages/app` receives a `ManagementClient` from either host. Desktop
also supplies local owner approval and Agent Tool placement capabilities.
Host callbacks provide the system-browser handoff for OAuth and local
Profile CLI recipes. Wails imports and generated bindings live in the
desktop host. The backend binding checks owner authorization independently
of which UI controls are available.

The desktop process does not start `menderd` and does not use the management HTTP API internally.

Phase 16 adds one deliberately narrow native integration: Provider plugin installation uses the Wails directory picker to select a local plugin package directory, then passes that host-local path directly to the Wails binding. The shared React app receives this as an optional capability, so the Web host does not gain arbitrary server-filesystem path access.

Local Sandbox creation also receives a desktop-only directory-picking capability. The folder action beside **Absolute local directory** opens the native Wails directory-only picker and fills the editable absolute-path field; cancellation preserves the current value, and picker failures are surfaced in the creation page. Manual paths remain valid, including in Web where the host cannot reliably expose a local absolute path. The existing Review step and Runtime authorization/path checks still govern Sandbox creation: picking a folder does not grant permissions or create a Sandbox.

## Database location

The desktop process resolves its default database under Go's per-user `os.UserConfigDir()`:

```text
<UserConfigDir>/Mender/mender.db
```

For development or isolated testing, set `MENDER_DB` to an absolute path in a
dedicated temporary directory. The default managed Skill and plugin roots are
siblings of the database. Native credentials still use the OS keyring.

```bash
MENDER_DB=/absolute/path/to/mender.db wails3 dev
```

In PowerShell:

```powershell
$env:MENDER_DB = 'C:\path\to\isolated-mender\mender.db'
wails3 dev
```

The parent directory is created before Runtime startup.

CLI and daemon database-path behavior is unchanged.

## Authenticated MCP Providers

Desktop shares Web's inventory-backed Provider `Authentication` workflow. HTTP authentication binds a Credential Reference to a header name/scheme; stdio authentication binds a Credential Reference to an explicit child environment name. Provider persistence contains References and formatting metadata only. Secret values are resolved at connection/process boundaries and required References are re-authorized before upstream Tool calls.

Legacy HTTP Provider header templates are still resolved from the **Mender desktop process environment** for compatibility. A typical legacy template is:

```text
Authorization: Bearer ${ANYSEARCH_API_KEY}
```

Only the legacy template is persisted. The resolved secret is not stored in the Mender SQLite database. This compatibility path is not represented by Profile Credential scope; new normal authentication should use inventory-backed bindings.

For local development, ensure the variable is present in the process that launches Mender, for example when using `wails3 dev`:

```bash
ANYSEARCH_API_KEY=... wails3 dev
```

On macOS, do not assume an environment variable that exists only in an interactive shell is available to an application launched separately through Finder/Launch Services. If a legacy header template or environment-backed Credential reports a missing variable, launch the development executable from an environment that contains it or configure the user session environment explicitly.

Native Credentials store secret material through the OS credential store. Mender's SQLite database stores only inventory metadata; there is no plaintext fallback if the native keyring is unavailable.

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

## Troubleshooting restricted-host Tunnel TLS

`wails3 dev` uses the same desktop Runtime process-isolation policy as a native production build. A visible `tunnel-client` process or a local `/readyz` response does **not** prove that the OpenAI control plane can poll. Find the client-local health/admin listener (the `--health.listen-addr` default is ephemeral for Mender), then check `/health?details=true`, `/api/status` (specifically `tunnel_metadata_error`), or `tunnel-client health --port <port> --require-control-plane-poll`. Do not print the child environment or secret-bearing logs.

On macOS, `TunnelClient=restricted_host` requires the narrow `com.apple.trustd.agent` Seatbelt Mach service lookup for Go's OS-backed x509 verification. The symptom before this exception is `tls: failed to verify certificate: x509: OSStatus -26276`, while an unrestricted Go probe succeeds and MCP initialize works. This is distinct from DNS anomalies, ChatGPT connector setup rate limits, or an upstream HTTP 401/403. Preserve certificate verification and sandbox isolation; never work around the error by disabling verification or exporting API keys. Rebuild/restart Wails and restart the managed server so its tunnel child inherits the corrected profile. Only after the client reports a successful control-plane poll should the ChatGPT connector registration be retried.

## Sandbox lifecycle confirmation in macOS WebView

The Wails v3 macOS `WKUIDelegate` in the pinned beta implements file selection, but does not implement `runJavaScriptConfirmPanelWithMessage`. In the shared Sandbox detail page, `window.confirm` therefore cannot be the approval gate for `sandbox.destroy` (or Docker Start, which discards scratch data): the button may appear to do nothing in the desktop app. The normal browser test environment supports `window.confirm`, so the existing UI tests alone did not catch this.

The management dialog uses a **single in-dialog confirmation step** instead. Start/Stop remain immediate when permitted; Destroy and Docker Start first show the pending action and consequences, then require a separately labeled confirmation button. Cancel/Back does not call the backend; errors appear in the same dialog. The backend retains authorization, owner, generation, active-lease, and Workspace Provider reference checks. A successful Local Workspace destroy **tombstones** the run for audit and does not remove the inventory row or delete host files. An unavailable backend or active execution lease is never bypassed.

When validating Desktop: create a disposable local Sandbox in a test database, verify Cancel issues no RPC, confirm Destroy issues one RPC and yields `destroyed`, and verify the new record remains viewable while the associated host root is untouched. Do not destructively test against a user's existing Sandbox to validate the UI.

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

Native Wails build, packaging and WebView interaction require separate platform
verification. See the real-environment acceptance matrix (historical).

## Product scope

Desktop currently exposes the same management workflows as the Web UI:

- Tools services, Catalog search/describe/invoke and Workspace targets,
- Profiles,
- managed MCP Servers from Profile delivery,
- editable PolicySet / Rule management,
- Credential inventory/create/rotate/delete,
- Audit.

Policy management is shared with Web through the generated Wails contract. Desktop exposes the same persistent PolicySet/default/Rule CRUD, validates Wails string enums at the TypeScript host boundary, and uses the same three-step Decision → Scope/Targets → Governance Facts editor with inventory-backed Profile/Provider/Tool selectors. Rule order is never presented as priority, and `require_approval` accurately reports the current fail-closed Approval behavior.

Credential management is shared with Web. Native secret values appear only as
Create/Rotate method arguments. Profile ToolBinding management provides separate
Presentation, Governance and Credentials actions. Provider connection and
authentication have separate dialogs. The Wails adapter maps normalized
Credential References, and TypeScript never resolves secret bytes. Desktop Tool
calls use the same canonical Invocation outcomes as management HTTP, including
`rejected` and `failed`. Audit exposes minimized origin, scope and outcome
metadata.

The Profile's MCP delivery page configures mode, Tunnel and direct projection,
then starts or stops its loopback Streamable HTTP server. Running servers stop
with the Desktop process. Requests refresh projections from the current
Profile, MCP delivery and Catalog revisions. stdio serving remains
client-launched through the CLI and uses its startup snapshot.

Plugin Marketplace manages installations. Desktop can pick a local package
directory. Tools configures external Provider services from the persisted
schema and can rebind a Provider to another enabled installation of the same
PluginID while preserving ProviderID. Web shares inventory, configuration and
rebinding. See [External Provider Plugins](provider-plugins.md).
