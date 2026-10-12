# Native OpenAI Tunnel Health — Stuck Starting Regression

Date: 2026-10-12
Scope: Mender built-in OpenAI Tunnel transport; local desktop development mode.

## Observations and diagnosis

- A managed MCP HTTP listener is running and responds on its loopback /mcp endpoint (GET returns 405 by design; **not** an MCP protocol verification).
- The associated `tunnel-client` child process stays alive and exposes a separate loopback health listener.
- Read-only native checks against the live child returned `GET /healthz → 200 live`, `GET /readyz → 200 ready`, and `tunnel-client health --require-control-plane-poll → result ok`. `GET /health?details=true` reported `components.control-plane.status=ok`, `state=polling`, a recent successful poll and no consecutive failures. **This is evidence of local daemon/control-plane readiness, not proof a remote ChatGPT connector has been registered or that external MCP calls work.**
- Mender's own `tunnel/infrastructure/process/runner.go` discarded process stdout/stderr, bound `--health.listen-addr=127.0.0.1:0`, and returned a hardcoded `TunnelHealth() = "starting"`. Mender's `watchManagedTunnel` faithfully copied that constant every 400 ms. As a result the UI could never transition to ready, even while the native daemon was ready.

## Intent contract

**Outcome:** Mender truthfully projects the live native Tunnel health and control-plane poll state, without changing Profile IDs, credentials, tunnel registration, outbound network behavior or existing MCP listener admission.

**Invariants:**

1. **Never infer ready from PID or successful `exec.Start`**. A valid loopback native health report and at least one observed *successful* control-plane poll are necessary. Optional `cloudflared` status does not gate control-plane polling if disabled.
2. Health reads are passive HTTP GET against a host-selected `127.0.0.1` port, without credentials, proxy inheritance, headers, logs, process restarts or external network requests. Responses must be limited, validated and subject to short timeouts.
3. `tunnel-client` takes an OS-selected ephemeral loopback port. The Mender host reserves a port, closes the reservation directly before launch and passes that exact address to `--health.listen-addr`. This avoids trying to inspect an undiscoverable `:0` port and also avoids shared health-url files that can be hidden inside a Linux restricted-host /tmp mount. A failed bind or health timeout degrades rather than reporting ready.
4. Keep a distinct `starting` grace period (bounded); after repeated failures or absence of control-plane proof, report `degraded`, not eternal `starting`. If child exits, the established MCP watcher reports `exited`; do not automatically relaunch or silently relax isolation.
5. Credentials remain injected via `CONTROL_PLANE_API_KEY` in child environment only and are erased from the parent `exec.Cmd` immediately after Start. Health error messages never include raw response body, headers or secret material.
6. Startup remains async and the management plane reads cached health only. Stop closes child and background watcher, avoiding goroutine/port leaks and persisting no health files.
7. Do **not** modify the user's currently active MCP Server or Tunnel Client process during development. Updated Go backend needs an intentional Wails dev restart/restart-server to execute; Vite hot reload alone cannot replace the already running native Go process.
8. Tests use a synthetic local HTTP health service and fake launched child; no real model, credential, tunnel registration or paid API call.

## Staged acceptance

- **Doc commit:** root cause, observation versus confidence, implementation and security gates.
- **Implementation commit:** dynamic loopback port assignment, bounded status sampler, monotonic/fail-closed state transitions, process lifecycle and tests. Include successes, missing control-plane poll, degraded status, invalid/malformed/unreachable endpoint, process exit, and no secret in cached health.
- **Final verification:** `go test -race` for the native Tunnel and managed MCP packages plus `pnpm check`; verify UI projection via existing managed MCP tests. Report real process restart or GUI smoke only if actually performed.

## Non-goals

No new tunnel/control-plane protocol, no API key rotation, no automatic retries/restarts, no log retention of sensitive data, no assumption that native readiness equals remote ChatGPT registration, and no compatibility bridge for arbitrary health formats.

## Closure evidence (2026-10-12)

- **Implementation:** `cdb7189` adds an OS-assigned, host-known loopback health port, bounded HTTP sampling of the native `GET /health?details=true` schema (no proxy, redirects, arbitrary URLs, or credentials), a recent successful control-plane poll requirement, and cached `starting` / `ready` / `degraded` / `exited` phase transitions. Stop cancels the owned process group when supported by the Runtime launcher.
- **Synthetic process tests:** native process fixture transitions through `starting → ready → degraded → exited`. Negative tests cover absent health, failed control-plane state, stale success, bad/missing JSON and oversized bodies, untrusted redirects, process lifetime and cleared `exec.Cmd.Env`. Managed MCP server integration verifies its public TunnelStatus projection follows the reporter state. None of these tests touch the user's real tunnel or credentials.
- **Actual local evidence (read-only):** active native `tunnel-client 0.0.15` returned HTTP 200 for `/healthz` (`live`), `/readyz` (`ready`), and `tunnel-client health --require-control-plane-poll --json` (`result: ok`). Its control-plane reported `status: ok`, `state: polling`, and no consecutive failures; optional `cloudflared` was disabled. The user's *current* local MCP endpoint also answered a read-only `initialize` request with HTTP 200 and a valid MCP server response. This verifies local transport and control-plane polling, **not** ChatGPT connector registration or external tool invocation.
- **Repository gate:** `pnpm check` returned exit code **0** after the implementation changes, covering Go test/race/lint, SDK/protocol checks, Wails binding parity, TypeScript/ESLint, 174 Vitest UI tests, i18n, and Web/Desktop frontend builds.
- **Operational restart boundary:** while writing and validating code, the existing Wails dev process and its live tunnel-client remained untouched. The old Go backend holds the hardcoded `starting` implementation in memory; a frontend-only Vite refresh is insufficient. To observe the new phase collection, the operator must intentionally stop and relaunch `wails3 dev`, then start the Profile MCP server again. That restart temporarily disconnects the existing tunnel and must not be performed silently.

**Residual limitation:** the native health endpoint proves a currently healthy local daemon and control-plane poll, not that ChatGPT lists the connector or can successfully invoke a remote Tool. Validate that from the external client separately after restarting.
