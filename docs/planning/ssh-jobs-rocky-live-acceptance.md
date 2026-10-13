# SSH Durable Jobs — live Rocky acceptance and short-invocation budget

- Date: 2026-10-13
- Scope: operator-approved read/write smoke tests on the already configured Rocky Linux 9.8 SSH Sandbox; no Wails/MCP/Tunnel restart, no arbitrary system changes, no Core RemoteTask.

## Observed evidence before any fix

1. The current `default` Profile had the `job.submit`, `job.observe`, `job.cancel` Sandbox grants, and one `sandbox.jobs` Suite + 5 canonical Tools, but **zero** corresponding Profile ToolBindings. The owner explicitly approved binding the five Jobs Tools. They were added using `mender --db <existing-desktop-db> profile add-tool default <tool-id>` (existing local owner application use case) and read-back confirmed all five with inherited Policy. No other ToolBinding or grant was changed.
2. An already running ChatGPT MCP session still returned `MCP tool is outside profile` for the newly attached Tools; that is a **stale MCP Profile snapshot**, not a failed Profile grant or SSH credential. Restarting the Wails app/Tunnel is outside this acceptance's authorized scope.
3. Existing Rocky Workspace Shell confirmed root login, Rocky Linux 9.8, `systemd 252`, `/usr/bin/systemd-run`, `systemctl is-system-running=running`; no private Jobs store existed initially.
4. First live `jobs.submit` via the governed **local CLI** timed out at the standard 30-second Provider Invocation budget. The newly created remote plugin-owned record `sjob_29deeed6015ee33f63cd35f5a7038e1d` has a private `manifest.json` but no `run.sh` or terminal `exit` artifact and no corresponding systemd service. `jobs.status` returns `unknown`. The host execution lease was released; the permanent Job obligation remains. DO NOT retry without the original idempotency key, automatically delete the record, fake a terminal result or assume the shell command ran.
5. Cross-network existing-job `jobs.status` took approximately 20.6s and a nonexistent TaskID status took approximately 14s, leaving far too little margin inside the ordinary 30-second Provider timeout. By contrast, the current code explicitly gives **SSH Workspace** an implicit 60-second timeout, subject to the configured authoritative Runtime Budget (currently 120s).

## Accepted narrowly scoped fix

- Preserve the existing SSH Workspace 60-second implicit Tool deadline and generic Provider 30-second deadline. Add a **90-second implicit deadline only for an exact, host-verified, configured SSH Jobs Provider** (`io.mender.provider.sandbox-jobs`, version 1, pinned SSH Sandbox, reviewed source keys). User-supplied tighter timeouts, context cancellations, concurrency and the global Runtime Budget still win; the remote systemd job's separate four-hour maximum runtime does not change.
- The timeout must apply consistently to local CLI `CallConfiguredTool` and MCP `InvokeTool` without any new transport protocol or SSH retry shim. Tool classification is metadata-only; never open SSH or read credentials while classifying.
- Add positive and negative tests for the 90-second allowance and unchanged Workspace/generic behavior; `pnpm check`, security and targeted race checks. No fake or automatic replay after ambiguous acknowledgments.
- Retest with a newly generated idempotency key, short read-only logs/results and one harmless bounded cancellation, observing the first old task only. If a remote systemd or private task writer still fails, report it explicitly and preserve artifacts for investigation.

## Verification journal

_Pending scoped code and live checks._
