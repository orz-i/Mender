# Phase 24 Plan — Runtime Process Governance Closure

Status: Complete

## Objective

Close the remaining native-process governance exception by routing OpenAI `tunnel-client` through the same host-owned isolation boundary as Workspace shell and external Provider plugins, while hard-cutting the persisted arbitrary executable override and full host environment inheritance.

Architecture is governed by ADR-0034.

## Contract claims

### SC-1 — Tunnel is an explicit process class

RuntimeIsolation includes independent WorkspaceShell, ExternalProvider, and TunnelClient modes. TunnelClient is persisted and follows the same explicit `host | restricted_host` fail-closed semantics.

### SC-2 — Tunnel executable is not resource configuration

Tunnel configuration contains no executable field. Existing rows migrate forward while preserving Tunnel identity and references but dropping the executable override. No runtime compatibility field/parser remains.

### SC-3 — Tunnel launch is host-owned

The Runtime resolves the fixed `tunnel-client` executable to an absolute host path and launches it through the shared process-isolation launcher. Tunnel infrastructure does not call `os/exec` directly.

The child receives a bounded environment rather than `os.Environ()`. Only the runtime API key and explicitly host-owned non-secret values required by the process contract are supplied.

### SC-4 — Typed management parity

Management HTTP, OpenAPI/generated SDK, Wails/Desktop, Web/Desktop Runtime UX expose TunnelClient isolation and remove Tunnel executable configuration in one hard cut.

### SC-5 — Existing lifecycle/security properties remain intact

Managed MCP stays loopback-only. Profile server start/stop/restart/rollback still owns the Tunnel child lifecycle. Runtime Credentials remain references at rest and resolved secret material only at launch.

## Implementation clusters

### Cluster A — Documentation

Add ADR-0034, this phase plan, ADR index, and roadmap entry before implementation.

### Cluster B — Runtime/Tunnel model hard cut

- add `TunnelClient` to Runtime isolation domain/application/persistence;
- add forward Runtime governance migration;
- remove Tunnel executable from domain/application/persistence;
- add forward Tunnel migration that rebuilds rows without executable;
- add focused migration/domain tests.

### Cluster C — Governed Tunnel process launch

- add a host-owned Tunnel process launcher/resolver boundary;
- resolve fixed `tunnel-client` to an absolute path;
- use RuntimeIsolation.TunnelClient and the shared process-isolation backend;
- use a read-only canonical executable-directory permitted root;
- remove full host environment inheritance;
- preserve child stop/watch/rollback semantics;
- prove behavior with focused process/bootstrap tests.

### Cluster D — Contracts and management UX

Hard-cut HTTP/OpenAPI/generated SDK/Wails/shared React contracts:

- `RuntimeIsolation.tunnelClient` is required;
- Tunnel create/projection has no `executable`;
- Runtime isolation page exposes Tunnel client mode;
- Tunnels page removes executable override input/help text;
- English/Chinese i18n and focused tests remain aligned.

### Cluster E — Verification and closeout

Run focused tests throughout, then full generated-contract drift checks, Go lint/test/race, frontend lint/typecheck/tests/build, security scans, `pnpm check`, and `pnpm security:check`. Record final evidence and commit completion docs separately.

## Defaults

Runtime isolation:

- Workspace shell: `restricted_host` when available at first initialization, otherwise `host`;
- External Provider: `host`;
- Tunnel client: `restricted_host` when available at first initialization, otherwise `host`.

Persisted restricted-host settings never silently fall back when backend support disappears.

## Persistence migration

Runtime governance advances from schema v2 to v3 by adding required `tunnel_client` isolation state initialized from the same backend-aware default used for fresh Runtime creation.

Tunnel persistence advances from schema v1 to v2 through a forward-only table rebuild that removes `executable` and copies only canonical Tunnel fields.

There is no dual-read, dual-write, deprecated API field, or executable fallback.

## Verification scenarios

1. fresh Runtime isolation initializes TunnelClient to backend-aware default;
2. persisted TunnelClient setting round-trips and restricted_host remains fail closed when unavailable;
3. Runtime governance v2 database migrates to v3 deterministically;
4. Tunnel v1 rows migrate to v2 without executable while preserving IDs/references;
5. Tunnel domain/application no longer accept executable configuration;
6. Tunnel runner cannot launch when host executable resolution fails;
7. launcher receives an absolute resolved executable and read-only canonical permitted root;
8. selected TunnelClient isolation mode reaches the shared process-isolation backend;
9. Tunnel process environment does not contain unrelated host environment variables;
10. resolved API key reaches the child through `CONTROL_PLANE_API_KEY` and remains absent from persisted/projected state;
11. managed MCP Tunnel start/stop/restart/rollback behavior remains correct;
12. non-loopback MCP listener/Host/Origin protections remain unchanged;
13. HTTP/OpenAPI/SDK/Wails contracts require TunnelClient and contain no Tunnel executable field;
14. Web/Desktop Runtime page edits Tunnel client isolation;
15. Tunnels UI contains no executable override;
16. generated artifacts do not drift;
17. full repository/race/security gates pass;
18. `.agents/`, `.pi/`, and `.intent-loop/` are absent from commits.

## Non-goals

- network egress policy;
- automatic Tunnel client installation/update/signing;
- Tunnel readiness polling;
- new Tunnel Policy actions;
- remote end-user Principal propagation;
- interactive approvals;
- canonical Outcome budget and Unknown Fact policy changes.

## Commit strategy

1. `docs(governance): define runtime process governance`
2. `feat(runtime): govern tunnel client process`
3. `feat(app): hard cut tunnel process contracts`
4. `docs(governance): complete phase 24`

## Completion

Phase 24 is complete.

Implemented in segmented commits:

- `a7ac3df` — document the Runtime process-governance hard cut before implementation;
- `3974d53` — add TunnelClient isolation, forward-only Runtime/Tunnel migrations, remove persisted executable authority, and route Tunnel launch through the shared host-owned process launcher with bounded environment;
- `8165822` — hard-cut OpenAPI/generated SDK/Wails/Web/Desktop contracts and management UX.

Final properties:

- every current Runtime-owned native process class (`workspace.shell`, external Provider plugins, and Tunnel client) enters a host-owned launcher boundary;
- Runtime isolation independently governs WorkspaceShell, ExternalProvider, and TunnelClient;
- TunnelClient defaults to `restricted_host` when the backend is available and otherwise explicit `host`;
- Tunnel rows/API/SDK/UI no longer contain an executable override;
- old Tunnel executable data is removed by forward schema migration rather than preserved behind a compatibility parser;
- the host resolves the fixed `tunnel-client` executable to a canonical absolute path immediately before launch;
- Tunnel children receive a bounded environment containing the runtime API-key handoff rather than the complete host environment;
- managed MCP remains loopback-only and its Tunnel start/stop/restart/rollback lifecycle remains intact;
- secret Credential material remains outside persistent/projection state.

Verification completed on 2026-10-06:

- focused Runtime governance, Tunnel migration, process-launch, and managed MCP lifecycle tests;
- full `go test ./backend/...` during implementation;
- `pnpm check`, including Provider Plugin protocol/SDK/example checks, generated SDK/Wails drift checks, desktop architecture check, Go lint/test/race, ESLint, TypeScript, i18n, 71/71 frontend tests, and Web/Desktop production builds;
- `pnpm security:check`: zero reachable Go vulnerabilities and no high-or-higher production npm vulnerabilities (the primary Go vulnerability DB was temporarily unavailable, so the configured Google storage mirror was used);
- hard-cut source scan found no production direct `exec.Command` in the Tunnel package and no Tunnel executable field in current typed contracts/UI;
- `.agents/`, `.pi/`, and `.intent-loop/` remained outside all commits.

The remaining governance audit items—canonical Outcome byte budgeting/Unknown Facts, then verified remote Principal propagation and interactive approval—remain follow-up phases rather than being folded into this process-governance change.
