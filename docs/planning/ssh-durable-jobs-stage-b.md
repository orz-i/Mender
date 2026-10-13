# SSH Durable Jobs — Stage B

- Status: **Implementation closed (2026-10-13)** — automated gates passed; live Rocky/systemd acceptance remains an operator-only follow-up
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

- **Backend owns execution:** First-party `sshremote` contains job identity/protocol, a 0700 on-host task directory **adjacent to (not inside)** the pinned Workspace root (`<root>.mender-ssh-jobs-v1/<sandboxId>/`), exclusive directory claim (atomic idempotency), 0600 command script, `systemd-run --unit` and `systemctl` observation. This prevents structured Workspace file/read/search/edit tools from accessing job scripts and logs; a root Shell login can still read them because **SSH root is not OS isolation**. A running job is never represented by a host-held SSH Session Lease. Commands/outputs stay remote; Mender audit/diagnostics never store their plaintext.
- **Job identity and persistence:** `sjob_` identity derived from exact sandbox identity and a high-entropy caller-generated submission key; the remote directory is immutable after first reservation, and stores only the data needed to prevent replay and recover result/logs. If reservation exists but request or launch outcome cannot be verified, return `unknown` and never start another unit. `systemd-run` UnitID is stable and bounded; retained output/result files, not journald or transient unit state, are authoritative.
- **Lifecycle without global TaskStore:** The host persists only a per-Sandbox `jobs_managed` obligation/fence (no TaskID, no job status). SQLite v13 makes this marker immutable and the single exclusive execution lease serializes owner-authorized Job submit against stop/destroy across Runtime processes. Before Stop/Destroy, if enrolled, it first obtains an exclusive local durable fence against new submissions, connects to the *same pinned* backend, obtains an explicit plugin observation that every known job is terminal, and atomically commits the Sandbox Run CAS and releases that fence. Missing/unreadable store, ongoing/uncertain jobs, unavailable host, active Workspace lease or identity drift must fail closed; failed checks release only their own fence without modifying Sandbox state. Do not infer that an empty `systemctl` result implies no tasks.
- **No cross-layer bypass:** A Job submission must first hold the governed, exact Sandbox temporary acquire/fence before remote reservation. Stage B may expose an internal host-only seam for deterministic tests but no externally callable Tool/HTTP or Profile grant; Stage C must add its own approved job-specific admission instead of abusing `process.shell` as equivalent authorization.
- **Task cancellation:** Persist a cancel-request marker before asking systemd to stop; report `cancelRequested`, not `cancelled`, until the remote wrapper reports terminal outcome. A forceful systemd kill can prevent a final marker: maintain `unknown` rather than fabricating a terminal state. Unknown remote termination is not reclassified as cancelled. Cleanup/retention is explicit and cannot be a side effect of ordinary status GET.
- **Limits:** In B the runner requires an SSH login as **root** and the system-level `systemd-run` manager; unprivileged `systemd --user`/linger is not implemented or inferred. Every job has a **4-hour systemd RuntimeMaxSec**, 64 KiB maximum command input and per-log-chunk read, 64 TaskID reservation slots per Sandbox (including permanent idempotency tombstones), 64 process tasks and 1 GiB unit memory bound. Slots intentionally do not self-expire: until a separate, auditable retirement/idempotency-expiration contract exists, submissions fail closed once the quota is reached. Unknown/partial states cannot be silently reclaimed. Remote file operations reject symlinks where checked, and task-root 0700/file 0600 mode is required. Remote storage assumes integrity of the configured login and its host; root access provides **no OS isolation**. Server power-loss durability depends on remote filesystem behavior, not just SFTP acknowledgements; host reboot does not cause automatic Job execution restart.
- **Compatibility:** Existing Workspace operations, Tool IDs, Profile grants, SDK/CLI/MCP contracts, `sandbox.workspace` binding, external Sandbox Plugin Protocol v1 and Plugin Registry built-ins remain unchanged. B is not an advertisement of operational `sandbox.jobs` until Stage C supplies governed tools. An unclean Mender crash can strand a short-lived `sandbox_execution_leases` row; generic leases have no automatic expiration and require reviewed operator recovery. This deliberately favors safety over silent replay, and Stage C must preserve a read-only, independently authorized observation path even while a prior lease is uncertain.

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

## Stage B closure evidence (2026-10-13)

| Package | Commit | Verified output |
| --- | --- | --- |
| B-doc | `f866960` | Contract / trust and execution ownership documented **before** code |
| B-engine | `856ab62` | SSH plugin-owned immutable TaskID, private remote record and script, `systemd-run` adapter, status/logs/result/cancel and explicit unknown semantics; local SSH/SFTP fake-runner acceptance |
| B-fence | `060ece1` | SQLite v13 **only** persistent per-Sandbox SSH job obligation, exact-owner short submission lease, cross-Runtime lifecycle fencing; task state stays in plugin-owned remote storage |
| B-verify | This documentation closure | Full checks and remaining operator-only limits below |

**Full automated gates:** `pnpm check` **exit 0**, including the generated Provider/Sandbox/Tunnel protocols and examples, module isolation, generated API/Desktop/SDK parity, Go lint/tests/**race**, ESLint, strict TypeScript, **i18n 1167/1167**, **32 Vitest files / 202 tests passed**, and Web/Desktop builds. `pnpm security:check` **exit 0**: no reachable Go symbol vulnerability, no known high-severity production JavaScript dependency vulnerability (one indirect Go module issue not called by application symbols). `cd apps/desktop && go build ./...` **exit 0**. `git diff --check` passed. Pre-existing Vite JS chunk-size and macOS linker deployment-target warnings are non-blocking; the generated standalone Desktop test binary was removed.

**Specific verified acceptance:** first Submit with a unique key reserves one SFTP directory and cannot replay an uncertain launch; a second concurrent invocation on a separate authenticated SSH connection does not start again; command-digest mismatch conflicts; ACK loss, partially written manifests and missing terminal records report `unknown`; cancellation never claims success before a durable terminal artifact; byte-cursor logs and output persistence recover on a new SSH connection **after the original transport is explicitly closed**. Symlink/invalid ID, oversize command and private-file permissions fail closed. Non-root accounts cannot select B's system-level Runner. SQLite stop/destroy races with first submission and a held lease across independent Runtime connections were exercised under race tests, including an immutable obligation marker, a fenced state CAS and crash/restart persistence of the marker.

**External acceptance is NOT claimed:** no real Rocky Linux host, real PID1 systemd/systemd-run transient unit, SELinux enforcement, systemd journal retention, root authentication, SSH private key, OS reboot, packaged running Wails app or live Tunnel was used. The `JobRunner` process-lifetime test uses test-owned local child processes behind a real local SSH/SFTP fixture; it proves detachment from the SSH transport, not acceptance by Rocky's actual systemd manager. Before advertising `sandbox.jobs` to Profiles, a separate operator-approved Rocky smoke should verify a bounded real job and cancellation, including private file labels and remote storage location.

**Durability/availability limits:** only systemd **system** manager with root login (not `systemd --user`); `RuntimeMaxSec=14400s` (4 hours) does not imply reboot resume or Checkpoint; maximum 64 per-Sandbox TaskID tombstones without automatic retirement; unknown/partial jobs and stale generic execution leases favor explicit operator reconciliation, not automatic TTL expiry, replay or force cleanup. The on-host 0700 sibling store is not a second security sandbox; root Shell can access it. No externally available `sandbox.jobs` Tool Suite or MCP Tasks adapter exists yet. Existing Workspace/Provider/ToolID/Grant interfaces and External Sandbox Protocol v1 are unchanged.

**Stage C handoff:** provide a separately admitted, optional `sandbox.jobs` suite with submit/status/logs/result/cancel, stable ToolID and Profile/Policy/Audit; do not treat `process.shell` authorization as equivalent to a detached job grant. Add read-only job observation independent of a stuck short-lived lease; provide an explicit, reviewed owner recovery path for crash-held leases, and a bounded tombstone-retention/expiration policy before claiming indefinite submission capacity. Expand versioned capability declaration only after the typed execution and authorization path is verified. Native MCP Tasks mapping is a separate client-negotiated follow-up. A managed Browser, Docker Job or Kubernetes Job never inherits SSH-specific systemd semantics.

**Git and operator state:** no running Wails/MCP/Tunnel restart, real remote SSH operation, credential import/read, or updates to `.agents/`, `.pi/`, `.intent-loop/`. Only these three original directories remain untracked. All Stage B implementation source was committed before this closure log.
