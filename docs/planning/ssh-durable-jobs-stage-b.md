# SSH Durable Jobs — Stage B

- Status: In progress (2026-10-13)
- Governing contract: [ADR-0040](../architecture/adr/0040-optional-plugin-capability-contracts.md)
- Scope: SSH Remote backend-owned durable job engine and a narrowly bounded Sandbox lifecycle fence; no Tool Suite exposure or MCP Tasks mapping.

## Intent contract

**Outcome:** Long-running work submitted through an explicitly governed SSH Remote backend can outlive an SSH session and Mender process; the original SSH plugin owns TaskIDs, authoritative remote records, observations, cancellation and cleanup. Sandbox stop/destroy never silently strands plugin-managed work. Mender stays a Tool/Resource Aggregation Platform, not a Remote Task Platform.

**Required checks:**
1. B1 — Exact SSH backend and resource identity, backend-owned `sjob_` TaskID and idempotency key bind an immutable command digest; retry with same key does not repeat side effects, conflicting payload fails. Uncertain commit/launch never automatically retries, including restart.
2. B2 — A short-lived SSH connection submits a bounded systemd transient service with stable unit identity, timeout and resource limits. The service continues detached; remote on-disk task record and append-only bounded log readers survive connection and Mender restarts. No session holds a Mender execution lease for job lifetime.
3. B3 — `status`, `logs` with byte cursor, `result` and `cancel` use the same TaskID identity, report `unknown` if inaccessible/ambiguous, and never confuse cancel-request acknowledgment with termination. Terminal metadata is authoritative only when a durable result artifact is present; unit garbage collection, host reboot, journal rotation cannot fabricate completion.
4. B4 — A scoped, persistent **host lifecycle obligation marker** (NOT Task table) is installed prior to the first admitted remote submit; it survives restart and makes SSH stop/destroy prove absence of nonterminal or uncertain plugin-owned tasks. A held fence serializes the proof with admission/new submit and SQL state transition; offline/unknown is fail-closed. SSH Sandboxes not enrolled into Jobs preserve existing passive lifecycle semantics.
5. B5 — Fake/test-owned SSH/Systemd or port-backed fixtures prove idempotency under ACK loss, concurrent duplicate submits, status recovery, logs cursor, cancellation and lifecycle racing with submissions. Negative tests cover path/symlink/owner/limit failures and cannot activate without admission. Full `pnpm check` and security/race when feasible.

**Non-goals:** Globally persisted RemoteTask, task scheduler, task DB migration, AgentTask reuse, automatic remote reboot restart or checkpointing, true exactly-once remote side effects, cross-plugin generic Job RPC, `sandbox.jobs` Provider/Tool Suite, Profile delegated job grant, MCP Tasks, browser Docker/Kubernetes job execution. Real Rocky host operations, credential reads, Wails/Tunnel restart require a separate operator choice; this phase uses local isolated fixtures.

## Architecture and security decisions

- **Backend owns execution:** First-party `sshremote` contains job identity/protocol, dedicated on-host private workspace under the pinned root, exclusive directory claim (atomic idempotency), command script, `systemd-run --unit` and `systemctl` observation. A running job is never represented by a host-held SSH Session Lease. A remote command runs with the configured login's actual privileges (especially **root is not isolated**). Scripts and task records may contain sensitive commands: keep remote permissions strict and never log them into Mender audit/diagnostics.
- **Job identity and persistence:** `sjob_` identity derived from exact sandbox identity and a high-entropy caller-generated submission key; the remote directory is immutable after first reservation, and stores only the data needed to prevent replay and recover result/logs. If reservation exists but request or launch outcome cannot be verified, return `unknown` and never start another unit. `systemd-run` UnitID is stable and bounded; retained output/result files, not journald or transient unit state, are authoritative.
- **Lifecycle without global TaskStore:** The host persists only a per-Sandbox `jobs_managed` obligation/fence (no TaskID, no job status). Before Stop/Destroy, if enrolled, it first obtains an exclusive local durable fence against new submissions, connects to the *same pinned* backend, obtains an explicit plugin observation that every known job is terminal, and atomically commits the Sandbox Run CAS and releases that fence. Missing/unreadable store, ongoing/uncertain jobs, unavailable host, active Workspace lease or identity drift must fail closed; failed checks release only their own fence without modifying Sandbox state. Do not infer that an empty `systemctl` result implies no tasks.
- **No cross-layer bypass:** A Job submission must first hold the governed, exact Sandbox temporary acquire/fence before remote reservation. Stage B may expose an internal host-only seam for deterministic tests but no externally callable Tool/HTTP or Profile grant; Stage C must add its own approved job-specific admission instead of abusing `process.shell` as equivalent authorization.
- **Task cancellation:** Persist a cancel-request marker before asking systemd to stop; report `cancelRequested`, not `cancelled`, until the remote wrapper reports terminal outcome. Unknown remote termination is not reclassified as cancelled. Cleanup/retention is explicit and cannot be a side effect of ordinary status GET.
- **Limits:** Bounded command bytes, small result/log chunk, bounded concurrency and retained task count, remote file/command checks, no symlink traversal. Remote storage assumes integrity of the configured login and its host; if `root` is used there is *no OS isolation* and malicious remote users with the same identity may tamper with it.
- **Compatibility:** Existing Workspace operations, Tool IDs, Profile grants, SDK/CLI/MCP contracts, `sandbox.workspace` binding, external Sandbox Plugin Protocol v1, plugin Registry built-ins remain unchanged. B is not an advertisement of operational `sandbox.jobs` until Stage C supplies governed tools.

## Work packages and bounded commits

| Package | Deliverables | Gate |
| --- | --- | --- |
| B-doc | This source-based contract / threat model and exact ownership | Docs-only commit before implementation |
| B-engine | Backend-owned remote Job protocol, scripted systemd runner, immutable idempotency, safe TaskStore, status/logs/result/cancel, deterministic test-double and SSH fixture | Race-tested job semantics, zero User/Rocky I/O |
| B-fence | Tiny host-owned persistent Sandbox Job obligation and admission/transition fencing, no Task table | Cross-Runtime concurrent submit vs stop/destroy; no probe for unenrolled SSH; fail closed on uncertainty |
| B-verify | Regression/security/full gates and explicit evidence; close Stage B | `pnpm check`, `pnpm security:check`, tracked tree clean, explicit commit hashes |

## Source baseline

- `sandbox/infrastructure/sshremote/backend.go`: `Session` opens pinned SSH+SFTP, Workspace operations only; cancellation retains uncertain lease. Do not reuse that uncertainty behavior to hold long jobs.
- `sandbox/infrastructure/sshremote/config.go`: 45s bounded pinned-host dial and SFTP rooted checks; `Connection` is short lived.
- `sandbox/infrastructure/sqlite/leases.go`: single token per Sandbox, no TTL; release occurs after session close. Must serialize with job admission/fence.
- `sandbox/infrastructure/sqlite/lifecycle.go`: Stop/Destroy checks no active lease then CAS; currently does NOT know about detached jobs.
- `bootstrap/sandbox_ssh_remote.go`: SSH Stop/Destroy currently changes local binding without remote work, must retain that default when no job usage exists.
- `backend/internal/toolsuite/*`: `sandbox.jobs` unavailable and unregistered by design until Stage C.
- `sdk/sandboxplugin` Protocol v1: `ExecuteWorkspace` only, unchanged.

## References

- systemd-run transient units, --unit / --service-type / --property / --no-block: https://man7.org/linux/man-pages/man1/systemd-run.1.html
- systemd.service Type=exec and restart/exit state: https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html
- systemd unit GC means task results must be separately persisted: https://manpages.debian.org/bookworm/systemd/systemd-run.1.en.html

## Closure log

_To be completed after implementation and verified checks._
