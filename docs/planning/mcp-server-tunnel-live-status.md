# MCP Server Tunnel Status — Live UI Reconciliation

Date: 2026-10-12
Status: In progress (focused follow-up to `native-tunnel-readiness-regression.md`)

## Report and cause

After the native Tunnel health sampler landed in commit `cdb7189` and the Wails dev backend was rebuilt, the Servers screen still showed a long-lived red `starting` badge.

Read-only investigation of the **current** Wails process (new native binary, current tunnel-client process, and a new loopback MCP port) confirmed that the child daemon's `/health?details=true` reported `live:true`, `ready:true`, `control-plane.status:ok`, `state:polling`, and a recent `last_success`; `tunnel-client health --require-control-plane-poll` returned `result:ok`. This is local/control-plane readiness, **not** proof of a remote ChatGPT registration.

The additional defect is in `packages/app/src/domains/servers/servers-page.tsx`: the `servers` array is populated once on page mount and is replaced after an explicit Start/Restart/Stop or manual Refresh. Tunnel Start deliberately returns the initial `starting` status before async Go health polling observes `ready`. Even though Go `watchManagedTunnel` updates the cached `TunnelStatus` approximately every 400 ms, the React screen never reloads that cached state. The badge also renders every status other than `running` as destructive red, including benign `starting` and verified `ready`; raw `tunnelStatus` strings bypass static i18n.

## Intent and implementation boundary

Fix the **UI's passive status projection**. Reuse the existing `MenderClient.listMCPServers()` via Web HTTP or Wails bridge. Do **not** add Tunnel health HTTP calls to the browser, a second backend watcher, server endpoint probing from the UI, or any native restart.

1. While the Servers screen is mounted and at least one configured Tunnel-backed MCP Server is present, poll the canonical Mender server inventory at a bounded interval (2 seconds). Monitor ready->degraded->exited transitions as well as starting->ready. Stop polling when none are present, when the page unmounts, or while a management operation is in progress.
2. Poll **servers only**, not Profiles/Credentials or Tunnel control plane. Keep the existing explicit Refresh button. Keep the background refresh silent: no busy spinner or clearing unsaved inputs; transient transport read errors must not erase the last known rows or claim readiness.
3. Prevent responses that started before a user-triggered Refresh/Start/Restart/Stop from overwriting newer results or reviving a removed server. Use a monotonically increasing operation epoch and unmount/cleanup fencing; do not allow overlapping polls.
4. Represent statuses honestly: pending/starting is neutral, ready is non-error, failed/degraded/exited are negative. Provide localized static labels rather than displaying English enum literals on a Chinese screen.
5. Preserve server IDs, Tunnel IDs, Profile configuration and all process lifecycle semantics; no backend/API/database changes. Wails dev frontend HMR should apply without restarting the running Tunnel.

## Acceptance

- Regression test: mock an initial Tunnel-backed server with `starting`, then a later inventory with `ready` followed by `degraded`; ensure the page updates without clicking Refresh and that only `listMCPServers` is polled.
- Race test: a poll in flight when the user explicitly stops/restarts a server cannot overwrite the resulting list.
- Unmount/empty-list test: no polling remains after leaving Servers or after the last server disappears.
- Verify i18n, TypeScript, shared Web/Wails UI parity and `pnpm check`.
- Do not claim screenshots, remote ChatGPT connectivity or real Wails UI clicks unless actually verified.

Follow-up is bounded to the present bug; do not reopen completed native transport orchestration.
