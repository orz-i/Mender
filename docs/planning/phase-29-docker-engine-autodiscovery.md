# Phase 29 — Docker Engine automatic local discovery

Status: **Closed (2026-10-11)**. Supersedes the Phase 28 *manual desktop connection* UX only; ADR-0035/0036/0037, Router, Policy, Lease and immutably pinned Docker templates remain authoritative.

## Intent and audited baseline

A Mender Desktop user should not have to click “Connect Docker Desktop” each launch. Docker is a trusted **built-in** `sandbox.backend/main` extension, whose existing `ActivateDocker` already uses the inspected local Docker Engine, validates socket and daemon security capabilities, and registers backend + lifecycle atomically. Marketplace read is inert. Without `MENDER_DOCKER_SOCKET`, it currently reports `unavailable` and waits for a manual Wails call.

The previous manual consent UI is explicitly replaced by the user's instruction to automatically discover and connect Docker Engine; this does **not** imply permission to create/start containers automatically, expose the daemon to models, or relax the current resource/provisioning gates.

## Contract and invariants

1. **Only trusted native Desktop auto-discovery**. On app startup, a host-owned bounded worker tests a fixed, ordered list of known local UNIX socket paths for the current OS account. On macOS: `~/.docker/run/docker.sock` (Docker Desktop), `~/.colima/default/docker.sock` (Colima), `~/.orbstack/run/docker.sock` (OrbStack). Linux: current-user Docker Desktop/rootless sockets and `/run/docker.sock`. Skip nonexistent entries early, do not follow symlinks, Docker contexts, `DOCKER_HOST`, Docker CLI, TCP/SSH/remote endpoints or arbitrary files. A socket must pass the **existing** canonical-path/permissions + live Engine Linux/cgroup-v2/seccomp/cpu/memory/PID probe before registration. First successfully verified engine wins; never silently retarget a live Sandbox or switch engines during the same Runtime.
2. **Safe asynchronous recovery**. Desktop startup is not gated on a slow or offline daemon. A single cancellable/retry-bounded worker probes immediately and periodically (10 s) if no engine is connected, so launching Docker Desktop after Mender eventually enables the Marketplace card. It stops after a successful activation, on app shutdown, or when explicitly configured `MENDER_DOCKER_SOCKET` takes precedence. No unbounded subprocesses; no container/image mutations during probing; no endless concurrent attempts. Failure remains `unavailable` with a useful diagnostic.
3. **Read-only catalogue and UX**. `GET /plugins` and `GetSandboxBackends` remain inert snapshots. Marketplace and Docker create selection refresh current backend status at a bounded cadence while Docker is unavailable, so the card changes to Enabled and the focused **Create sandbox** flow unlocks without a manual connection dialog. Delete the obsolete desktop ConnectDockerDesktop callback/binding/confirmation rather than leaving a hidden compatibility path. Headless `menderd` and CLI still require explicit host-owned `MENDER_DOCKER_SOCKET`; web remains unable to choose a daemon or trigger discovery.
4. **No new privileges or side-effects**. Automatic daemon connection confers no model/Tool ability to select a socket, pull images, create a Sandbox, mount host paths or bypass Principal/Policy/Approval/lease/budget/audit. Docker's local immutable digest and strict resource plan remain unchanged. Do not present “connected” as continuous health certification: the status represents a prior successful guarded probe, and daemon shutdown may cause operations to fail closed.
5. **Test gates and closure**: test deterministic candidate list/ordering and rejection of untrusted homes or socket shapes; worker retry, cancellation and no duplicates; provider/Router integration against a real Docker daemon using the existing live E2E and pre-cached digest; Wails/Desktop + HTTP statuses, auto UI refresh, no manual privileged action; `pnpm check`, `pnpm security:check`, native desktop Go build, and smoke of dev app. Do not touch user-owned containers or databases for tests.

## Work packets and commits

- A — this document/acceptance only.
- B — shared host-only fixed candidate discovery and single cancellable worker + unit/live tests.
- C — Desktop startup integration, remove manual Wails connection and focused UI tests; verify full gates and close.

No speculative subphase proliferation, Docker context scanning, public registry, background image downloads, cloud endpoint or new Agent loop. Never stage `.agents/`, `.pi/` or `.intent-loop/`.

## Relevant official references

- Docker Desktop on macOS UNIX socket: https://docs.docker.com/desktop/setup/install/mac-permission-requirements/
- Docker Desktop on Linux UNIX socket: https://docs.docker.com/desktop/troubleshoot-and-support/faqs/general/
- Colima socket: https://colima.run/docs/faq/
- OrbStack Unix socket: https://docs.orbstack.dev/faq
- Docker daemon security: https://docs.docker.com/engine/security/protect-access/

## P4 — Docker Sandbox creation UX defect (user-reported, 2026-10-11)

The initial auto-discovery activation is healthy and real Engine tests provision Docker containers, yet the Sandbox create form only exposes a free-text `Immutable image digest`. A user with locally cached `alpine:3.22` sees no discoverable images; a normal tag does not satisfy the form, disabling progress. This is a **resource-selection UX defect**, not a license to accept mutable tags as persistent targets.

Acceptance: expose a strictly read-only, bounded **locally verified immutable image inventory** on authenticated HTTP and Wails/SDK; use the same Engine probe as Sandbox creation; canonicalize Docker Hub's short `RepoDigests` to fully qualified `docker.io/...@sha256` and verify every selection against the observed image. The focused Docker form offers a selection of installed images that fills the original `imageDigest` field, retaining advanced manual Digest entry only when needed. No image tag is persisted, no pull/build/removal, no daemon-config option and no Tool-facing Engine list capability. No Docker runtime => empty/fail-closed inventory and no accidental Sandbox creation. Test real cached images on Desktop, API/SDK/Wails parity, denial/empty/not-found states and UI selection end-to-end before closure.

## Closure evidence — 2026-10-11

Status: **Closed**. Phase 29 includes the user-reported Docker creation image-selection correction.

- `7a9c56d`: document host-only ordered socket discovery, retry/cancel and hard gates.
- `2dc1a7e`: implement fixed Unix endpoint candidates, read-only Engine probe,
  exactly one cancellable worker, bounded retry and atomic exact Registry binding.
- `870d45c`: native Desktop startup auto-discovery, Marketplace/create state refresh,
  removal of manual Wails connection callback and confirmation.
- `cb11eb8`: document why mandatory free-text manifest Digest caused apparent
  inability to create a Docker Sandbox even when the Engine was ready.
- `8e89529`: verified read-only local Docker image enumeration, safe Docker Hub
  digest normalization, strict image identity checks and 64-option maximum;
  typed HTTP, Wails and TS SDK responses, negative and live integration gates.
- `313a8ac`: focused Docker image selector that persists the exact image digest,
  advanced manual digest fallback and corresponding UI/i18n acceptance.

Verification: `pnpm check` passed (Go lint/test/Race, protobuf, API/SDK/Wails
binding drift, TypeScript lint/typecheck, i18n **957/957**, **148/148** React
tests and Web/Desktop builds); `pnpm security:check` found no known Go or
high-severity production JS issues. Native macOS Desktop Go build passed with
existing deployment-target linker warnings. Real Docker Engine 29.8.0 on
macOS accepted automatic startup discovery; real cached Alpine digest
`docker.io/library/alpine@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8`
appeared in the host's immutable image inventory. Both real HTTP Docker create/
stop/start/destroy and Docker Workspace parity/isolation live tests passed with
`-race`. All test resources were ephemeral and uniquely labeled; no existing
user Sandbox, container or database was deleted.

Outstanding release-specific check: visual click-through on signed/packaged
Wails app is not asserted by these automated gates; production application
packaging/signing remains a release verification concern. No runtime image
download or mutable `tag` target has been introduced.
