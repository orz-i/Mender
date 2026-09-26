# ADR-0011: Isolate Wails as a desktop shell with a host-neutral management client

- Status: Accepted
- Date: 2026-09-26

## Context

Phase 6 established an OpenAPI-first loopback management API, a typed browser SDK, and a reusable React product application in `packages/app`.

The desktop distribution now needs to reuse that product UI while invoking the local Mender Runtime directly. Making product pages import Wails bindings would couple shared UI to a single host. Starting `menderd` inside the desktop app merely to reuse HTTP would add an unnecessary local network boundary and duplicate process/server lifecycle concerns.

Wails v3 is still prerelease software. The reviewed current release for Phase 7 is `v3.0.0-beta.26`, released 2026-09-25. Wails documents v3 as Beta while describing the API as stable.

The CLI and daemon must continue to build and test without native desktop/WebView dependencies.

## Decision

Mender will treat Wails v3 as a desktop-only shell and binding transport.

The desktop shell is isolated in its own Go module at `apps/desktop`, with module path `github.com/orz-i/Mender/backend/desktop`. The module imports the existing backend Runtime/Application composition and pins `github.com/wailsapp/wails/v3 v3.0.0-beta.26`.

The main backend Go module does not depend on Wails. CLI and daemon import graphs must not include Wails packages.

The desktop process constructs one local `bootstrap.Runtime` directly and closes it during Wails shutdown. It does not launch or call the management HTTP server.

A desktop binding adapter delegates to the same Runtime/Application use cases used by other adapters. Wails SDK types and generated binding types do not enter Domain or Application packages.

On the frontend, `packages/sdk` owns an explicit `MenderClient` interface representing the management capabilities consumed by `packages/app`.

The browser host satisfies `MenderClient` with the existing OpenAPI/openapi-fetch HTTP implementation.

The desktop host satisfies the same `MenderClient` with generated Wails bindings.

`packages/app` receives only `MenderClient`; it does not import Wails runtime APIs or generated bindings.

The browser host uses `BrowserRouter`. The desktop host uses `HashRouter` so embedded WebView navigation does not require history-route fallback from the Wails asset server.

Wails production assets are built with Vite and embedded into the desktop binary with Go `embed`.

The desktop database defaults to `<os.UserConfigDir()>/Mender/mender.db`, with `MENDER_DB` as an explicit development/test override.

No tray, notifications, deep links, file dialogs, or other native integrations are added in Phase 7 without a concrete product requirement.

## Alternatives considered

### Run menderd inside the desktop application and reuse HTTP

This would let the browser SDK work unchanged, but it introduces a loopback server, port ownership, HTTP lifecycle, and local network attack surface that the desktop process does not need.

### Import Wails bindings directly from packages/app pages

This minimizes adapter code but makes the product UI desktop-specific and breaks the Web/Desktop host boundary established in ADR-0005 and ADR-0010.

### Put Wails in the main backend Go module

Go only compiles imported packages, so CLI binaries could technically remain Wails-free. A separate desktop module makes the dependency boundary stronger and auditable: the backend module itself does not acquire Wails/native GUI dependencies.

### Create a second desktop React application

This would avoid abstraction work but duplicate Providers, Catalog, Profiles, Policy, Credentials, and Audit workflows and allow host behavior to drift.

### Add native integrations while the shell is being introduced

Wails supports many native features, but adding unused capabilities increases platform-specific code and security surface without solving a current product need.

## Consequences

### Positive

- Web and desktop use the same product pages.
- Desktop calls the Runtime directly without a local HTTP hop.
- CLI/server remain independent of Wails and native WebView dependencies.
- Browser and desktop transports are replaceable behind one frontend contract.
- Runtime policy, credential, invocation, and audit semantics remain centralized.
- Wails prerelease churn is contained to the desktop shell and generated bindings.

### Negative / trade-offs

- The management client contract must remain structurally compatible with both OpenAPI and Wails bindings.
- Desktop requires its own transport adapter and binding-generation check.
- The repository has a second Go module and Wails Task/build configuration.
- Hash URLs are visible in desktop WebView navigation state, though they are not user-facing in normal use.
- Phase 7 packaging is not equivalent to a signed production release; signing/notarization is deferred to Phase 8.

## Follow-up

Phase 7 implements the desktop module, binding adapter, shared frontend client contract, native build scaffold, and boundary tests.

Phase 8 defines production signing, notarization, checksums, SBOM/security scanning, and upgrade/distribution policy.
