# SSH Durable Jobs — abnormal recovery and Sandbox lifecycle research

- Date: 2026-10-13
- Status: **Proposed / research only**. No remote records, locks, SQLite rows, running services or execution state were changed by this research.
- Context: [ADR-0040](../architecture/adr/0040-optional-plugin-capability-contracts.md), [Stages B/C](ssh-durable-jobs-stage-b.md), [real Rocky and ChatGPT MCP evidence](ssh-jobs-rocky-live-acceptance.md).
- Decision owner: the Mender project. Nothing in this document authorizes clearing the live partial TaskID.

## 1. Executive decision

**Keep SSH Job recovery inside the SSH Sandbox backend plugin.** Mender Core only owns authorization, exact resource binding, short-lived dispatch/lifecycle fences, Run generation/CAS, grant/Policy/audit and a resource-level obligation that the backend still has unresolved work. There is no universal RemoteTask, Scheduler, Agent loop, task DB table or job reconciliation daemon in Core.

Split three orthogonal questions:

1. **Execution outcome:** Did the command complete, fail or get cancelled? This is established by backend-owned, verified durable execution evidence. Absence of proof is `unknown`, not `failed`.
2. **Current execution activity:** Is the *managed systemd task* still alive or potentially able to execute? `active`, `quiescent_proved` and `uncertain` are **evidence verdicts**, not execution outcomes. They require proof that the dispatcher cannot still start a process.
3. **Sandbox admission/lifecycle:** Can Mender accept another submit, stop binding new work, or destroy the resource association? This is owned by Mender Core; it consumes a fresh plugin-issued quiescence proof under a durable exclusive host fence.

**Never `unknown → failed` just to release a resource, delete the plugin's idempotency tombstone, retry an uncertain command automatically, or infer no execution merely because a transient Unit has disappeared.**

## 2. Audited current execution and actual incident

`sshremote.Jobs.Submit` runs a strictly ordered series of operations:

```
Host: authorize (tool.invoke + sandbox.acquire + Profile job.submit)
    → pinned SSH Open → prepare private root
    → SQLite ReserveSSHJobSubmit: exact Ready generation + short exclusive lease
       and permanent sandbox_ssh_job_obligations marker IN ONE COMMIT
Remote: mkdir <root>.mender-ssh-jobs-v1/<SandboxID>/<TaskID> (exclusive)
    → manifest.json (exclusive; includes immutable command digest and unit ID)
    → run.sh (exclusive; embeds command and terminal artifact trap)
    → systemd-run --collect --service-type=exec, fixed unit identity
Host: SSH session close + short lease release; job thereafter owned by plugin
Remote: process stdout/stderr, exit.tmp → rename to exit
Read: Status checks manifest, optional cancel/exit, then systemctl when exit absent
Stop/Destroy: exclusive lifecycle fence → backend Quiescent scan → CAS and fence consumption
```

The initial 30-second Tool budget on the real Rocky SSH host expired **after `manifest.json` but before `run.sh`**, leaving `sjob_29deeed6015ee33f63cd35f5a7038e1d`. Later Status returned `unknown`, the private directory had no script or exit file, and no managed Unit was found; the short SQLite lease was released, the permanent resource obligation remained. The 90-second exact SSH Jobs Tool invocation budget fix (`7fb4101`) and subsequent CLI/ChatGPT MCP execution, idempotency, log cursor and cancel checks passed; that does **not** rewrite this earlier record.

**What the incident suggests:** By the current first-party v1 source ordering a dispatcher cannot normally call `systemd-run` without first successfully writing `run.sh`. It is a strong *prelaunch* candidate, not presently a formal remote non-execution proof: no script *now* is not proof it never existed; an old writer may resume after a delayed packet, or remote root may mutate files. Any v1 operator resolution must verify submission owner/process provenance, remote metadata integrity and a fenced inability of a delayed old request to launch.

Current source touchpoints:

| Concern | Implemented source | Current behavior/gap |
| --- | --- | --- |
| Idempotency and state | `sandbox/infrastructure/sshremote/jobs.go` | TaskID derived from SandboxID + 128-bit caller key; SHA-256 command digest; terminal only on verified `exit`; no recovery verdict/decision API |
| Private store | `sandbox/infrastructure/sshremote/jobs_store.go` | SFTP exclusive files and task dirs, immutable manifests; directory count includes 64 permanent tombstones; does not record dispatch-intent/boot ID/worker start |
| systemd runner | `sandbox/infrastructure/sshremote/jobs_runner.go` | `--collect` unloads transient Units; `Start` receipt only confirms systemd accepted service/exec, **not** durable user-command outcome; `Observe` cannot infer failed when Unit disappears |
| Termination | `sandbox/infrastructure/sshremote/jobs.go:jobScript` | SIGTERM trap and `exit.tmp → exit`; SIGKILL/host crash/forced service stop may suppress trap and leave no result |
| Exact recovery connection | `sandbox/infrastructure/sshremote/jobs_observation.go` | Read-only independent of the exclusive SQLite lease, safe for Status/Logs, but lacks evidence snapshot API |
| Core obligation | `sandbox/infrastructure/sqlite/ssh_job_fence.go` | Persistent immutable obligation per Sandbox, not Task table; acquired/released short lease has **no TTL**, process epoch or remote-writer fencing protocol |
| Lifecycle | `bootstrap/sandbox_ssh_remote.go`, `sandbox/infrastructure/sqlite/lifecycle.go` | Managed Stop/Destroy fail closed if even one job `unknown` or host offline; final CAS consumes exclusive fence atomically |
| Tool Facade | `bootstrap/sandbox_ssh_job_operations.go`, `sshjobs/infrastructure/plugin/*` | Separate submit/observe/cancel grants; `jobs.cancel` currently requires Ready; no owner-only inspect/reconcile API |
| Suite retirement | `bootstrap/sandbox_job_tool_suites.go` | Quiescent required for managed Jobs Suite retirement; job records and obligation intentionally kept after retirement |

## 3. Evidence classification matrix

| Observed backend condition | Execution outcome | Activity verdict | Automatic lifecycle clearance? | Proposed handling |
| --- | --- | --- | --- | --- |
| Verified immutable manifest + authentic `exit=0` | completed | quiescent | Yes, after fresh identity/record verification | Keep tombstone, preserve logs/result |
| Verified manifest + authentic nonzero `exit`, possibly `cancel=1` | failed or cancelled **only with specific proof** | quiescent | Yes, unless contradicting live manager evidence | Preserve correct reason; cancel-request alone is not termination |
| Unit active under exact pinned identity, no final exit | pending/running | active | No | Continue bounded observation or authorized cancellation |
| Directory or manifest incomplete | unknown | uncertain | **No** | Owner-only inspection, no automatic replay/delete |
| Manifest complete, script missing, no execution artifacts (the live v1 incident) | unknown / candidate `never_dispatched` | uncertain until writer/fence proof | **No, not today** | Targeted v1 **reviewed** `aborted_before_launch` resolution only after proof |
| Script exists, but no dispatch-intent marker (v1) | unknown | uncertain | No | Legacy v1 cannot retroactively prove no systemd launch |
| systemd manager offline or reports no unit and no `exit` | unknown | uncertain | No | `--collect`/reboot/journal GC are not negative proof of execution |
| `cancel` exists but result is absent | unknown | uncertain | No | Do not report `cancelled` based on stop acknowledgment |
| New v2 authenticated pre-launch evidence proves old writer fenced and launch was never requested | `not_started` operational disposition; **not** command failure | quiescent_proved | Yes, only via policy-approved resolution marker | Terminal tombstone still blocks replay of identical key |
| Command may have launched, process now absent, exit artifact lost | unknown | possibly inactive, **not proven** | No automatic clearance | Escalate to exceptional operator review/abandon with explicit residual risk |

**Important distinction:** A `recovery resolution` is **not** a fabricated command exit code. Prefer a backend-owned immutable `resolution.json` with verdict `aborted_before_launch`, evidence digest, identity/boot/attempt, actor and reason, while preserving original `manifest.json` and the fact the execution result was unknown. `Jobs.Quiescent` may recognize a strictly verified `quiescent_proved` resolution without reclassifying unknown as success or `exit=143`.

## 4. Proposed recovery protocol

### 4.1 Inspect / Plan — strictly read-only

Owner-only `InspectSSHJobRecovery(sandboxId, taskId?)` evaluates a **bounded plugin-owned inventory** and returns task phase, concise verdict, evidence freshness, evidence checksum, host reachability, relevant same-host boot ID/Unit identity, current Run generation and any held lease fingerprint. No raw shell command, script, SSH key, log content or full lease token is exposed to management views or audit. Never attempts re-execution or writes a status as a side effect of inspection. Profile `job.observe` keeps current safe read-only Tool behavior; *admin* inspection is not another model-facing Tool.

The UI initially displays a compact list/count: `running`, `terminal_proved`, `prelaunch_candidate`, `uncertain`. Selecting a record opens its evidence detail, then a separate review/confirm action (not an always-open form). The operator can inspect while a crash-held lease remains; no write lease is acquired by GET.

### 4.2 Resolve — explicitly requested, revalidated, fenced

Use existing local owner principal + `sandbox.reconcile` Policy/approval/audit or a narrowly reviewed dedicated operator action. Do **not** let a Profile's `job.submit`/`job.observe`/`job.cancel` grants authorize recovery, and do not expose `jobs.reconcile` through ordinary MCP until a separate review.

A resolution request names one SandboxID + backend TaskID, desired **operational verdict** (not an exit status), optimistic inspected evidence digest and a short reason. Under an **exclusive, short, SQLite-persisted Sandbox recovery fence**, re-read Run generation, binding, immutable host identity and evidence; reject stale digest, active writer, offline/ambiguous host, symlink/owner mismatch, partial or changed files, untrusted history or active Unit. If and only if plugin-side fencing **also** ensures no older dispatcher can launch later, append an immutable remote `resolution.json` using exclusive/atomic write + durability acknowledgement. Reinspect before acknowledging; release ONLY this recovery fence. All failure paths remain `unknown` with no hidden clearance. Preserve source artifacts and idempotency tombstones.

**Critical gap for v1:** A local SQLite random lease token does **not** fence an old SSH writer after lease loss. Before safe automatic `aborted_before_launch` recovery, add a backend-owned remote submission attempt epoch/ownership/fence checked **at the execution point**. For current v1 manifest-only tasks, allow only explicit human adjudication after verified old CLI/Wails writer shutdown and remote inspection; if that cannot be established, retain `uncertain` rather than unblocking. A timestamp or missing `systemctl` Unit is insufficient.

### 4.3 v2 remote journal — plugin-owned, not global TaskStore

Implement as a versioned private remote record layout, compatible through explicit v1 recovery rules, without modifying External Sandbox Plugin Protocol v1:

```
manifest.json         immutable TaskID / SandboxID / command digest / unit ID
reserve.intent.json   attempt ID, owner/run generation, protocol version, remote boot ID
run.sh                immutable script, hash-bound to manifest; prepared but not evidence of launch
launch.intent.json    fsynced BEFORE systemd-run; only one owner epoch may launch
launch.ack.json       manager acknowledgement, unit InvocationID / boot ID when available
start.json            worker-observed start after unit exec (when supported)
stdout, stderr        bounded log readers with byte cursor
exit.json             committed terminal evidence (exit / signal / reason / boot ID)
cancel.intent.json    durable request marker, not a terminal verdict
resolution.json       append-only explicit non-execution proof/owner adjudication
```

Journal files and their verification logic are the **SSH plugin's** responsibility. Write complete content to a private temp file, flush as supported (`fsync@openssh.com` or a reviewed remote helper), atomically rename where supported, verify permissions/identity and then issue ACK. A file `Close()` alone does not guarantee durability across remote power loss. No record is overwritten after an uncertain outcome; command digest mismatch remains a permanent conflict. For hostile/noncooperative `root` tasks, file ACLs cannot provide OS sandbox isolation.

`systemd-run --collect` may GC both succeeded and failed transient Units, so new journal artifacts must remain authoritative. `ExecStopPost` or a dedicated plugin-owned completion supervisor can improve terminal evidence after signals, but the source must prove exact exit semantics under SIGKILL, startup failure and power loss; do not assume all failures can always write a final file. Verify every proposed systemd property against the **actual Rocky systemd 252**, not only a newer online manual.

### 4.4 Stale Core short lease — separate failure class

The current `sandbox_execution_leases` row has a token, generation, principal/scope and timestamp, but no TTL, Runtime instance epoch or remote-writer guarantee. Its presence does not prove a live operation; its age does **not** prove an abandoned operation either. No `DELETE WHERE acquired_at < ...` or unsafe `force release`.

A future owner-only recovery path may capture an operation/Runtime instance generation, identify whether its original invocation is *definitively gone*, acquire a **new fencing generation**, have the remote backend reject stale launches or wait for a verified remote ownership barrier, then atomically take over/release the old lease with a separate audited reason. Without such a remote execution barrier, keep this as manual quarantine rather than claiming safe TTL-based takeover. Retain the original lease audit fingerprint and who confirmed its release; no Global RemoteTask aggregate is needed.

## 5. Sandbox lifecycle — three different admin operations

The current SSH Sandbox `Stop` is a **local admission authority transition**, not `systemctl stop` or remote OS shutdown. Mixing it with a hard requirement that all detached work must already be terminal can strand an owner with an offline host or unknown record. A safer lifecycle needs an explicit *disable-new-submissions / draining* distinction, not automatic task cancellation.

| Control / state | Submit | Observe existing tasks | Cancel under separate job.cancel grant | Normal Stop / Destroy | Invariants |
| --- | --- | --- | --- | --- | --- |
| Ready | Permitted after Policy/lease | Yes | Yes | Stop only after proof (current contract) | At most one short writer lease |
| **Drain / Pause admissions** (proposed) | **Denied immediately in Core**, including other Runtime processes | Yes | **Yes**, via special short lease on draining state | Stop after all terminal/resolved evidence | The drain fence is local/persistent, no remote connectivity required |
| Stopped | Denied | Allowed by existing owner/Profile observation | Currently **no**; if Stop changes to admission-only, must deliberately add safe cancellation | Destroy still requires fresh quiescence and no Provider/Policy references | Stop never implies remote process termination |
| Unknown or unreachable backend | Denied only after owner pauses admissions | Read-only local evidence / retryable remote observation | Only if backend reachable and authorized | No implicit Destroy | Remote uncertainty is explicit |
| Destroyed | Denied | Disabled under current facade | Denied | No normal reactivation | Destroy is final local resource tombstone, not implicit remote deletion |

**Immediate recommendation:** Keep existing fail-closed Stop/Destroy semantics while implementing read-only inspection, prelaunch recovery and an explicit **pause/drain** admission flag for SSH Jobs. Do **not** silently reinterpret Stop as killing remote tasks. A later accepted lifecycle contract may permit Stop as admission-only, but only after specifying observe/cancel availability for already detached jobs.

**Additional source-level risk to fix:** `sandbox/infrastructure/sqlite/lifecycle.go` Destroy checks the reference to `io.mender.provider.workspace` but does **not** equivalently assert absence of the `sandbox_job_suite_bindings` / `io.mender.provider.sandbox-jobs` binding. If all Jobs become terminal, this potentially permits a logical Destroy while a Jobs Provider remains bound; `RemoveSandboxJobTools` cannot then acquire a lifecycle fence on the Destroyed Run. Add an atomic check for **both** Workspace and Jobs Provider/Suite references in the Destroy CAS transaction, with a regression test. Separately ensure Jobs Suite retirement's Provider/Policy dependency check stays valid through actual removal (no check-then-delete race).

## 6. Retention, quota and boot recovery

The 64 record limit currently counts **every TaskID directory**, even terminal Jobs retained solely as replay-prevention tombstones; `PruneOutput` never frees those IDs. Time alone must not release an idempotency key. Separate *active/inspectable full records* from compact immutable digest tombstones, with an explicitly defined retention horizon and documented key reuse behavior. Do not advertise indefinite submission capacity or indefinite deduplication after an expiry policy unless the remote plugin can support both.

Remote host reboot is a distinct guarantee from SSH disconnect and Mender restart: transient systemd Units and volatile journald state cannot automatically resume tasks; boot ID can prove a particular prior-boot Unit is no longer running, **not** whether an arbitrary root shell command caused outside side effects before power loss. `unknown` remains honest; host reboot auto-resume and checkpoints are separate opt-in backend features, not Stage C defaults.

A real `systemd-run --service-type=exec` acknowledgment establishes a manager-side exec handoff for `/bin/sh`, **not** that the user command completed successfully. Prefer naming initial `submit` response `accepted/starting` or clarifying that current `running` is provisional until a separate worker/manager observation. Historical JSON/Tool schema compatibility should be handled explicitly, not silently.

### Priority order

| Priority | Concrete problem | Recommendation |
| --- | --- | --- |
| P0 | No owner-only evidence inspector; one `manifest-only` job permanently blocks Stop/Destroy | Add inspect-only path first, then deliberate v1 adjudication under a proven recovery fence |
| P0 | Destroy transaction does not check Jobs Suite references, allowing a potential orphan Facade | Add atomic Jobs + Workspace reference check and lifecycle regression test |
| P1 | Crashed short execution lease or post-launch missing result cannot be safely classified from timeout/GC | Remote writer fencing and explicit incident resolution; no TTL release or fabricated failure |
| P1 | No local admission freeze while an unknown job blocks normal Stop | Explicit owner-only drain/pause of new submissions, independently allowing observation and authorized cancel |
| P2 | 64 retained TaskID directories eventually exhaust submission capacity; no reboot resume guarantee | Compact tombstones, explicit retention contract, and separate opt-in host reboot semantics later |

## 7. Implementation proposal — one bounded Recovery stage, NOT a generic Job Platform

| Package | Ownership and deliverable | Acceptance gate |
| --- | --- | --- |
| R-doc | This proposal and the final accepted `Recoverability` + `Lifecycle` contract; decide evidence strength of v1 before touching the real incident | Document/source review; no change to current TaskID |
| R1 | SSH backend-only `InspectRecovery`, immutable versioned `launch.intent`, `resolution` and `QuiescenceProof`; owner-only non-mutating inspection first | Fake SFTP/real local SSH fixtures for every crash point; no automatic retries or false exits |
| R2 | Host policy/audit + exact Sandbox recovery admission/remote-writer fence + operation CAS; add Destroy guard for BOTH Suite bindings; expose owner-only compact evidence/confirm UI | Stop/Destroy vs submit/recover races across two Runtimes; forged IDs, expired proof, offline host, stale writer and concurrent drain fail closed |
| R3 | Safe v1 manifest-only **explicit** adjudication (if evidence adequate), capability-specific cancellation while draining, and persistent pause/drain of submissions; do not silently relax normal Destroy | Rocky read-only evidence before approval; manual mutation only with distinct authorization; no model-facing new Tool |
| R-close | `pnpm check`, security, Go race, desktop build; operator-approved Rocky acceptance; clean tracked tree, staged commits | Remaining unknown cases documented, not masked; backends beyond SSH unchanged |

Suggested package additions inside SSH plugin (naming only): `jobs_recovery.go`, `jobs_evidence.go`, `jobs_resolution.go`, plus minimal `sqlite/ssh_job_recovery_fence.go` and `bootstrap/sandbox_ssh_job_recovery.go`. No Core task DB, shared `RemoteTask`, Scheduler, Agent delegation or MCP Tasks is required. Do not over-expand into unrelated browser/Docker/Kubernetes job support.

### Deterministic fault-injection matrix

Test with the local pinned SSH/SFTP fixture and injectable Runner/SFTP failures: after directory creation; during incomplete manifest; after valid manifest but before script; while writing script; after fsync, before launch-intent; after launch-intent, before systemd ACK; ACK lost despite successful Unit; Unit garbage collected; terminal temp file not renamed; SIGTERM/SIGKILL; backend unreachable; SSH host reboot/boot ID change; corrupt/writable/symlinked task record; concurrent Submit same key; Reconcile vs late Submit; Reconcile vs Cancel; Reconcile vs Stop/Destroy; host process crashes leaving an exclusive lease; inconsistent or revoked Profile grant. Assert only evidence-backed terminal or lifecycle verdicts, no automatic re-execution, no fabricated exit codes, stable TaskIDs and bounded remote I/O.

## 8. Industry and protocol references (checked 2026-10-13)

- systemd-run, `--collect`, transient Unit garbage collection: https://man7.org/linux/man-pages/man1/systemd-run.1.html
- systemd.unit GC and `CollectMode`: https://www.freedesktop.org/software/systemd/man/systemd.unit.html
- systemd.service / `ExecStopPost`: https://www.freedesktop.org/software/systemd/man/systemd.service.html
- SQLite write/CAS transaction constraints: https://www.sqlite.org/lang_transaction.html and https://www.sqlite.org/wal.html
- OpenSSH SFTP extensions, atomic `posix-rename` and `fsync`: https://github.com/openssh/openssh-portable/blob/master/PROTOCOL
- Fencing stale lease holders: https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html
- MCP `2026-07-28` Tasks extension: https://modelcontextprotocol.github.io/ext-tasks/specification/2026-07-28/tasks.html — cancellation is only an intent acknowledgment, not guaranteed terminal cancellation. Mapping this extension remains a separate feature and does not transfer Task ownership to Core.

## Research disposition

**Recommend the bounded R recovery stage above only after the operator agrees to its lifecycle semantics and accepts that old v1 manifest-only records cannot be auto-resolved by a missing systemd Unit.** No source implementation, remote cleanup, TaskID resolution, unreconciled lease release, or running service changes were authorized by this research. The real old TaskID remains in `unknown` and must continue to fence normal Stop/Destroy until a reviewed disposition is implemented and approved.
