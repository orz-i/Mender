# Phase 28 — Docker Sandbox Marketplace activation

Status: Closed (2026-10-11). Governing ADR-0035/0036/0037; follows the closed Phase 27.

## Observed baseline

- Docker is **already** a real, host-trusted `sandbox.backend/main` implementation
  (`io.mender.sandbox.docker`), not a Provider or an external gRPC plugin.
  Its existing Engine security plan, immutable template, Execution Router,
  admission, leases and lifecycle cannot be reimplemented or downgraded merely
  to make a Marketplace card.
- `NewSandboxes` registers the Docker extension only after
  `MENDER_DOCKER_SOCKET` is present and a guarded daemon probe succeeds.
  The unified catalogue projects only registered backends, so Docker disappears
  from the Marketplace when not configured; the user cannot discover why or
  activate it from the macOS desktop.
- This host has Docker Engine 29.8.0 and a real Docker Desktop UNIX socket.
  `TestRuntimeDockerSandboxProviderEndToEnd` passed against the locally
  cached Alpine digest on 2026-10-11; read/search/edit/shell and the
  create/stop/start/destroy lifecycle were exercised on unique test-owned
  containers. No user-owned container was touched.
- The existing Plugin Marketplace is a local catalogue + native package
  installer, **not** an internet registry or public publishing service.
  A trusted built-in entry is the correct first-party listing in this scope.

## Contracts and delivery

1. **Discoverable but fail-closed**: Always include one stable Docker built-in
   entry (`io.mender.sandbox.docker@builtin`) in the canonical
   `GET /plugins` catalogue, including when the daemon is not configured or
   the probe is rejected. Its metadata comes from the same trusted built-in
   identity declaration. Distinguish `unavailable` from an administratively
   disabled installed package. Inert listing must not open a socket or start
   a container. No duplicate package record, counterfeit InstallationID or
   implied verified isolation.
2. **Explicit activation**: Offer an action on the Docker plugin detail in the
   local macOS Desktop **only**. It resolves the fixed Docker Desktop socket
   under the current OS user's home directory and requires operator
   confirmation. The backend validates canonical UNIX endpoint, socket
   permissions, Linux/seccomp/cgroup controls, and then registers the same
   trusted engine/backend/lifecycle with the Router. Never follow Docker
   contexts, `DOCKER_HOST`, untrusted Web inputs or network endpoints.
   Activation lasts for this Runtime process only; restarts require another
   opt-in or an explicit host-owned startup `MENDER_DOCKER_SOCKET`.
3. **One route, no fallback**: The detail page may navigate to the existing
   guided `/sandboxes/new?pluginId=io.mender.sandbox.docker` only after
   activation succeeds; it must not call an external Plugin gRPC driver or
   bypass Docker's immutable image digest and resource-limits review.
   Creation, Router dispatch, Profile grants and lifecycle remain unchanged.
4. **Parity and negative gates**: HTTP and Wails catalogue projections,
   Web/Desktop types, i18n, UI tests; unavailable-without-socket, bad/non-UNIX
   socket, concurrent/idempotent activation, exact registration and no-creation
   before opt-in; at least one live Docker E2E after implementation. Preserve
   the existing `MENDER_DOCKER_SOCKET` startup path for headless use.
5. **No side effects from discovery**: Reading or filtering the Marketplace
   never probes the daemon, pulls images, runs tools, creates containers,
   changes grants, or requests credentials. A failed probe must leave Docker
   unavailable, with an actionable diagnostic and no partial registry entry.

## Work packages / commits

- P0: this documented contract, no implementation.
- P1: stable, inert builtin catalogue and HTTP projection with precise
  unavailable state, backend regression tests.
- P2: Desktop-only explicit session activation and focused Marketplace
  confirmation; Wails/SDK/i18n/tests.
- P3: test gates, live Docker smoke and documentation; close without subphase
  proliferation.

## Boundaries

No external unsigned Docker native binary, no registry publishing claim,
no auto-connect or auto-pull, no native OS process confinement claim, no
`/var/run/docker.sock` symlink workaround, no unrestricted host mounts,
privileged/host-network containers or Agent-selected Engine access. Do not
modify `.agents/`, `.pi/`, `.intent-loop/`. Existing resources and their
immutable bindings remain untouched.

Docker socket access is highly privileged:
https://docs.docker.com/engine/security/protect-access/
On current Docker Desktop for macOS, the user socket is under
`~/.docker/run/docker.sock` and `/var/run/docker.sock` may not exist:
https://docs.docker.com/desktop/setup/install/mac-permission-requirements/

## P3 — verification and closure (2026-10-11)

- **Plugin identity**: `io.mender.sandbox.docker@builtin`, typed
  `sandbox.backend/main`. Unconfigured Runtime returns exactly one inert
  `unavailable` catalogue entry with four known Workspace capabilities,
  no installation/enable/create authority. The same key becomes `enabled`
  after Engine probe and atomic backend+lifecycle registration. No cloned
  driver, compatibility Provider or unsigned executable.
- **Operator flow**: Desktop Marketplace → Docker Engine → **Connect Docker
  Desktop** → review privileged-daemon warning → confirm → refreshed plugin
  detail → **Create sandbox**. Only Wails desktop binding has the connection
  method; it takes no browser-supplied endpoint. A fixed Docker Desktop socket
  (`~/.docker/run/docker.sock`) is vetted by existing guards. The connection
  is process-local and does not change startup configuration.
- **Failure**: absent/malformed/TCP socket or untrusted Engine leaves the
  adapter unregistered and the card `unavailable`. The ordinary HTTP
  management plane has no activation endpoint, and the generic installed
  plugin enable/delete actions do not apply to built-ins.
- **Tests**:
  - Go backend unit and race tests for Registry, catalogue and desktop host
    callback; HTTP `/plugins` projection asserts one stable Docker identity.
  - Marketplace/React/Wails tests cover dormant status, opt-in confirmation,
    cancellation, rejection, desktop-only action and activated create path.
  - Real Docker Engine 29.8.0 on macOS: both
    `TestRuntimeDockerSandboxProviderEndToEnd` and
    `TestDockerLiveWorkspaceParityAndIsolation` passed with `-race` using a
    pre-cached pinned Alpine image. Verified read/search/edit/shell,
    create/stop/start/destroy, immutable binding, non-root UID, read-only
    rootfs, no socket in container, `NoNewPrivs` and limits.
  - Go lint/tests/race, frontend lint/typecheck, 145 frontend unit tests,
    i18n 952/952, Wails generated bindings, module checks, Web/Desktop
    production Vite builds passed. macOS Desktop Go build passed (linker
    emitted platform-version warnings).
  - `pnpm security:check`: govulncheck (8 modules) and
    `pnpm audit --prod --audit-level high` found no known vulnerabilities.
  - `pnpm check`: passed after the final lint fix (see last commit).
- **Explicit limits**: This work publishes the built-in to Mender's local
  Plugin Marketplace, **not** a remote/public registry. Docker still requires
  an operator-owned daemon and a pre-cached immutable
  `registry/path/image@sha256:…` image, no automatic pull. Containers are
  constrained but Docker socket access is privileged; no VM-grade hostile
  multi-tenant security claim. Existing Wails dev processes must be restarted
  or rebuilt if they have not loaded the changed Go binary.
