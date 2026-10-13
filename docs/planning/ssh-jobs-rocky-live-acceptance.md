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

## Live Rocky acceptance journal

**Result:** Real Rocky SSH/systemd job execution, idempotency, status/results, byte-cursor logs, and confirmed cancellation **passed after the invocation timeout fix**. ChatGPT's previously established MCP server Profile snapshot did **not** reload the five newly authorized ToolBindings. One earlier manifest-only `unknown` job remains intentionally unreconciled and blocks Stop/Destroy until reviewed recovery.

| Check | Evidence | Outcome |
| --- | --- | --- |
| Original 30s failure | `sjob_29deeed6015ee33f63cd35f5a7038e1d`: immutable `manifest.json` exists under the plugin-private sibling store; no `run.sh`, no `exit`, no systemd Unit. Owner `jobs.status` returns `unknown`; active host lease count 0 | **Confirmed bug and preserved uncertainty**, no re-execution |
| Scoped timeout | `fix(ssh)` commit `7fb4101`: exact first-party SSH Jobs Provider only, 90s implicit budget, other Providers 30s, SSH Workspace 60s. Existing upper Runtime Budget clamps to 120s; user-specified tighter deadlines win | Scoped unit tests and lint pass; enabled live CLI submit |
| Real `submit` | New independent random key, `printf START; sleep 14; printf FINISHED` as root login, response `running`, systemd Unit `mender-sjob_135323fdb34b78e354bcce4dbc71ad69.service` | **Pass**: real remote systemd launch acknowledged |
| Detached result and reconnect | Separate subsequent CLI process invokes `jobs.status` and `jobs.result` for `sjob_135323fdb34b78e354bcce4dbc71ad69`: both return `completed`, `exitCode=0` | **Pass**, result persisted after original SSH session closed and transient Unit was garbage collected |
| Cursor logs | `jobs.logs(stdout,cursor=0,limit=15)` returns `MENDER_ROCKY_PR`, next cursor 15; second request returns remaining START and FINISHED lines, cursor 55, `truncated=false` | **Pass**, exact concatenated expected output |
| Idempotent replay | Explicit caller retry with **same exact key and command** returns **same** TaskID, `completed`, exit 0; no second systemd service launched | **Pass** |
| Payload conflict | Same key with **different command** returns canonical Provider failure; no new TaskID or side-effecting execution | **Pass**, conflict fails closed |
| Real cancellation | New task `sjob_18b5b6a700be16b153fdfe0298e37754` submitted `running` with a bounded `sleep 90`; explicit `jobs.cancel` returns `cancelRequested=true`, `state=cancelled`, `exitCode=143`. Separate later CLI processes confirm both `status` and `result` match; logs contain only `MENDER_CANCEL_PROBE_STARTED`, not the natural-end marker | **Pass**, cancellation terminal proof survived disconnect and reconnection |
| Remote terminal artifacts | Private dirs have mode 0700 and file modes 0600. The completed task has `exit=0`; cancelled task has `exit=143` and `cancel`. `systemctl list-units 'mender-sjob_*.service'` shows no currently active test service | **Pass**, no lingering test systemd Unit |
| Existing ChatGPT MCP | `mender_search_tools('job')` remains empty; `mender_describe_tool` for a newly bound ToolID returns `MCP tool is outside profile` on the already-open MCP session. DB confirms all five ToolBindings and three Job grants belong to `default` | **Needs an authorized session/server refresh**, *not* a Jobs execution failure |
| Reboot and platform-wide lifecycle | No Rocky reboot or real network fault injection. Old manifest-only `unknown` remains a permanently reserved identity under B's fail-closed semantics, so Quiescent cannot currently prove all jobs terminal | **Not claimed**; operator-reviewed recovery required before Sandbox Stop/Destroy |

### Operator safeguards

- All new live tasks used separately generated caller idempotency keys, saved on the operator's local machine outside Git. Neither key nor private SSH credentials are stored in this document or project commits.
- The original partial record was **not** retried, rewritten, falsely marked complete, or automatically removed. `systemd-run` was invoked only by the governed Jobs backend for the two new test tasks. Root command payloads only printed unique marker lines and slept. No business files, host configuration, system packages, or services were modified by test commands.
- No Wails, Mender MCP Server or Tunnel restart was requested or performed. The current live desktop runtime may require a refresh/restart to pick up the newly committed 90s implicit budget. The CLI smoke used a rebuilt binary from the committed source, not the stale already-open MCP session.
- The persistent `unknown` directory holds a quota slot and blocks Quiescent lifecycle. A future **operator-reviewed, evidence-based `aborted_before_launch` reconciliation** is required; do not delete the manifest or fabricate `exit=0`/`143` to force lifecycle success. This should be implemented separately from normal Jobs Tool supply, with explicit run/lease fencing and no side-effect replay.

### Scope of proven guarantees

This is a real *SSH session-detached* and *Mender process-reconnected* test: each Tool invocation opened and closed a distinct managed SSH transport, and on-host result files remained authoritative after systemd transient-unit collection. It is **not** proof of host reboot resume, lossless network fault tolerance during the submit ACK, checkpoint recovery, exactly-once external side effects, or new Tool exposure on the currently running ChatGPT MCP session.

## Security/audit and automated closure checks

- Host SQLite `invocation_audit` directly confirms the successful `jobs.submit` and `jobs.cancel` invocations each received `policy_kind=allow`, `approval=not_required`, `status=completed`, and `outcome_kind=complete`. The deliberately conflicting replay was audited as `status=failed`; the audit table contains no raw shell command or stdout column. The managed Sandbox has **zero** remaining active SQLite execution leases.
- Remote SFTP storage inspection showed **all three** test TaskID directories `0700`, manifests/scripts/output/exit/cancel files `0600`, and **zero** currently active `mender-sjob_*.service` units. The unreconciled original reservation has ONLY its `0600` manifest and cannot be mistaken for completed execution.
- `pnpm check` **exit 0** after timeout fix, including generated Provider/Sandbox/Tunnel contracts, SDK and Desktop binding checks, Go lint, all Go tests and **full race suite**, TypeScript strict, ESLint, **32 Vitest files / 202 tests**, **1178/1178** Chinese i18n coverage, and Web/Desktop frontend builds.
- `pnpm security:check` **exit 0**: no reachable Go symbol vulnerabilities; one indirect module advisory not referenced by application symbols; no known high severity JavaScript production advisory.
- `cd apps/desktop && go build ./...` **exit 0**: only pre-existing non-blocking macOS minimum target linker warnings. The generated standalone executable was removed; no running Wails/Desktop service was manually restarted. Vite still warns non-blockingly about >500kB chunks.
- Repo commits: documentation-first `0273876`, targeted timeout fix `7fb4101`, this acceptance evidence closure. All changes staged explicitly without `.agents/`, `.pi/`, `.intent-loop/` or user-owned secrets.

**Acceptance disposition:** SSH Jobs' real systemd-backed **execution / terminal result / logs / idempotency / cancellation** = **PASS** using the newly built governed CLI, and (after operator-controlled Wails/MCP/Tunnel restart and ChatGPT tools refresh) **PASS directly from ChatGPT MCP**. The initial persistent `unknown` task = **OPEN safety incident**, requires an explicitly reviewed recovery implementation or operator decision; do not silently clear it.

## ChatGPT MCP refresh — follow-up acceptance (2026-10-13)

**Operator action, outside assistant:** User restarted Wails Desktop, MCP Server and Tunnel, then refreshed the Mender tools in ChatGPT. The assistant did **not** restart or modify any of these live services.

| Check directly from the *refreshed ChatGPT Mender connector* | Observed evidence | Result |
| --- | --- | --- |
| Tool discovery & schema | `mender_search_tools` returns all five reviewed Jobs Tools (`jobs.submit/status/logs/result/cancel`); `mender_describe_tool` resolves each exact ToolID and input/output schema from `prov_f16a722561fef6af629a4f4f4c7a6c17` | **PASS**, prior `MCP tool is outside profile` is resolved |
| Old completed task | `jobs.status(sjob_135323fdb34b78e354bcce4dbc71ad69)` returns `completed`, exit `0` | **PASS**, restarted MCP observes durable historical terminal state |
| Old cancelled task | `jobs.result(sjob_18b5b6a700be16b153fdfe0298e37754)` returns `cancelled`, exit `143`, cancel requested | **PASS**, confirmed terminal cancellation survives MCP refresh |
| Old log recovery | `jobs.logs(...,stream=stdout,cursor=0,limit=256)` returns both prior probe START / FINISHED lines, next cursor `55`, not truncated | **PASS** |
| Direct ChatGPT submit | New randomly keyed benign root command `printf START; sleep 8; printf FINISHED` creates `sjob_b17bd46701bd8e2520f412454d0f4744` and acknowledged systemd unit, initially `running` | **PASS**, no local CLI fallback |
| Direct ChatGPT status / logs / result | Subsequent calls report `completed`, `exitCode=0`; returned both exact START / FINISHED markers, cursor `53`, `truncated=false`; `result` also returns exit `0` | **PASS**, output and result persisted independently of first SSH session |
| Direct ChatGPT idempotent submit | Same exact random key and command return **the same** `sjob_b17bd46701bd8e2520f412454d0f4744`, `completed`, exit `0` | **PASS**, no duplicate execution |
| Direct ChatGPT cancellation | New independently keyed `printf START; sleep 90; printf NATURAL_END` creates `sjob_2776851d9fe9085a2df5ba22229723b6` (`running`); `jobs.cancel` returns `cancelRequested=true`, `cancelled`, exit `143` | **PASS**, actually cancelled the running systemd-backed task |
| Post-cancellation recovery | Separate `jobs.result` returns `cancelled`, exit `143`; stdout only contains START with no NATURAL_END, cursor `28`, no truncation | **PASS**, durable termination not just a cancel-request acknowledgment |
| Remote OS proof | Rocky Shell read-only check reports **0 active `mender-sjob_*.service` units**, `exit=0` for completed ChatGPT task and `exit=143` for cancelled ChatGPT task | **PASS**, no new lingering transient units |
| Legacy uncertain reservation | `jobs.status(sjob_29deeed6015ee33f63cd35f5a7038e1d)` still returns `unknown`, and remote `exit` artifact remains missing | **OPEN**, deliberately not replayed, rewritten, falsely completed or destroyed |

**Updated scope:** The previously deferred ChatGPT MCP tool exposure and full five-Tool execute path are now **fully verified directly in ChatGPT** after the operator refresh. These results do **not** establish remote host reboot restart, machine-power-loss durability, a deliberately injected mid-ACK network failure, generalized exactly-once side effects, or audited reconciliation of the old ambiguous reservation. No SSH credentials, random submission keys, or private task command scripts were added to source control. No additional code fix was needed after the preceding `7fb4101` timeout patch.
