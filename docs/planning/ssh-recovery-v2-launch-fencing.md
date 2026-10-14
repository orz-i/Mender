# SSH Durable Jobs R3 — execution-point fencing and evidence-bound resolution

- Date: 2026-10-13
- Status: **design / acceptance gate; not enabled**
- Predecessors: ADR-0040, ADR-0041 and `ssh-jobs-exception-recovery-lifecycle-research.md`
- Resource boundary: SSH Backend owns TaskID, remote evidence, launch gate, resolution and result recovery. Mender Core remains a Tool/Resource aggregation, governance and supply platform; it owns Principal/Policy/Audit, explicit Profile capability grants, short resource access fences and Run-generation lifecycle CAS.

## Current proven foundation

- `dedc5f9` and `1e0ce33`: Destroy must reject existing Workspace/Jobs Provider/Suite references, and no post-Destroy provider or suite association may be created. State transitions and reference checks are serialized by SQLite.
- `a176eaa`: owner-only read-only legacy SSH evidence inspection, with private commands/logs/credentials absent from the response and no implicit reconciliation.
- `b48f84a`: owner-only persisted Pause/Resume, with `ReserveSSHJobSubmit` checking Pause atomically with its short execution lease across Runtimes; observation remains available and remote tasks are not stopped.
- `09be94a`: interrupted singleton Jobs facade can be repaired by explicit `Ensure` without generating a new ProviderID, ToolID or Profile grant.
- `523d8ca`: Go lint gate fixes. The project-wide `pnpm check` completed successfully on these changes.

**No v1 `unknown` Task has been resolved**, and neither `Inspect` nor `Pause` can currently prove that a late SSH writer will never launch a command. Normal Stop/Destroy remains fail-closed.

## Non-negotiable R3 execution safety

A local SQLite lease, wall-clock expiry, absence of `run.sh`, lack of a currently loaded systemd unit, SSH disconnect, and a machine boot-ID change are individually insufficient to prove a command never ran. A remote systemd Unit may be garbage-collected even after executing, and a delayed old dispatcher might still submit a unit.

For a **new, explicitly versioned v2 job**, execution-point fencing must establish both of these properties:

1. Before performing any user-controlled command side effect, a trusted backend-owned gate obtains the exact per-TaskID exclusive remote lock and **durably commits a launch/start-intent marker**, including TaskID, immutable manifest digest and writer attempt ID. If this commit cannot be proven, the gate MUST fail closed before running the command.
2. A separately authorized owner-only recovery operation obtains the **same remote lock**, verifies the exact Sandbox/TaskID, trusted configured host, pinned Run/generation, immutable prepared script/gate and absence of a committed start marker, then **durably commits an immutable operational resolution marker**. The v2 execution gate checks that marker **under the lock before any launch/start marker or user command**, including for a delayed prior `systemd-run` call. An already-running gate cannot be overtaken.

A resolution is `aborted_before_launch`, **not** a fabricated shell exit code. `JobState=unknown` can coexist with `Activity=quiescent_proved`; ordinary `jobs.status/result` must never silently report it as completed or failed. No idempotency key is reused or replayed after resolution.

A partial/missing/corrupt marker, unverifiable script gate, lost fsync, active lock holder, uncertain host identity, mismatched Run generation, missing mandatory capability, backend offline, or conflicting source evidence results in **no resolution** and no lifecycle release.

## Suggested plugin-owned v2 journal

Keep legacy v1 records immutable. Use a clearly versioned v2 manifest/layout for **new** jobs only; never infer v2 protocol from a plain capability string.

| Remote record | Purpose | Required ordering |
| --- | --- | --- |
| `manifest.json` v2 | TaskID, SandboxID, unit, command digest and gate/script digest | Immutable exclusive create and verified persistence |
| `run.sh` v2 | A backend-controlled lock-and-marker gate plus bounded command execution | Immutable, digest checked; never replaced by recovery |
| `launch.intent` | Dispatcher request/attempt evidence, prior to systemd-run | Durable before remote dispatch; absence alone is not no-execution proof |
| `start.intent` | Execution gate's proof it may begin user-command side effects | Durably committed while holding the lock, before exec |
| `exit` | Verified terminal exit/termination evidence | Must not be inferred from an absent/GC'd unit |
| `resolution.json` | Explicit, immutable `aborted_before_launch` with evidence fingerprint, actor, scope and reason | Written once while holding the same remote lock; checked at execution point |
| `stdout` / `stderr` | Bounded plugin-owned log cursor | Content never included in Core owner-inspection audit |

The remote gate MUST make write/persist/check ordering visible to testable code. A temporary file close is not persistence proof. Use only a verified filesystem and protocol with appropriate file/directory durability semantics. Require a verified `fsync@openssh.com` capability (OpenSSH extension) or an explicitly reviewed remote helper; there is **no silent fallback** to unflushed writes. Require and verify util-linux `flock` semantics on the actual Linux target, including lock FD lifetime (do not use `--close` while relying on a child holding the lock). A weak NFS/remote-filesystem lock is not sufficient.

Root shell commands can escape their cgroup or corrupt their own journal: no arbitrary-root command can claim VM-level containment or a malicious-remote-host absence proof. This R3 guarantee is conditional on an operator-trusted host and enforced first-party gate; it is not a generic exactly-once effect guarantee.

## Core and plugin handshake

1. Owner explicitly pauses new submissions (or a specific creation policy denies them) and calls an **owner-only review API**, not a Profile Tool.
2. Core performs `sandbox.reconcile` Policy/approval/audit admission, then reserves an exclusive **short** Recovery fence using exact Sandbox ID, matching generation, trusted Principal and current immutable resource binding. A stale Core lease must not be cleared based on age.
3. Plugin reads bounded, fresh v2 evidence and checks exact remote lock/marker/host identity. Core cannot supply arbitrary remote paths or shell code.
4. Owner submits an explicit targeted resolution with inspected evidence fingerprint, disposition, and nonempty reason. The executor **rechecks** all evidence while holding both fences. A changed digest or missing evidence is a conflict.
5. Plugin writes `resolution.json` through a durable atomic exclusive operation, reads back the marker and returns an evidence-backed result. A lost acknowledgment yields **unknown**, never automatic replay.
6. Core releases only its short Recovery fence and records minimized admission/decision audit. The permanent Sandbox Job obligation stays. Future Stop/Destroy calls still require a fresh plugin `Quiescent` proof, then consume a new lifecycle fence in the same Run CAS.

If any of these steps is unimplemented, do not register a `resolve` route or expose an action that suggests safe clearance.

## Test/acceptance matrix: mandatory before enabling v2

| Fault or race | Expected result |
| --- | --- |
| After TaskID reservation, before manifest | Unknown, no replay, inspectable |
| Manifest written, script missing or corrupt | V2-only recovery may resolve iff the lock/gate can provably exclude late user execution; v1 remains unknown |
| Script prepared, `launch.intent` absent | Revalidate script gate; if recovery commits first, any late dispatcher must be blocked by marker |
| Launch request sent, ACK lost | No resubmission; if script already committed `start.intent`, recovery is denied |
| Script gate holds lock before `start.intent` | Concurrent resolve cannot overtake gate |
| Gate committed `start.intent`, command killed, no exit | Remains unknown, no fabricated exit or lifecycle clearance |
| Resolution lock obtained before delayed systemd unit begins | Unit's later gate sees resolution, does not execute user command |
| SFTP fsync unsupported, remote directory not durable, lock unavailable or host offline | Fail closed; no v1 fallback |
| Profile caller, wrong owner, mismatched TaskID, stale Run, policy deny | Reject before secrets, SSH or marker modification |
| Pause/Resume vs submit/resolve/Stop/Destroy across two Runtime processes | No accepted new submit after committed Pause; no Destroy with outstanding references; no double-resolution |

All local regression tests must use disposable SSH/SFTP fixtures and fault-injected fake runners; no real user remote resources, root shells, keychain entries or existing unknown TaskIDs are mutated. Before claiming a real supported v2 backend, the operator must separately approve a **throwaway** Rocky Linux/systemd 252 acceptance target and verify exact `flock`, `fsync`, filesystem and systemd-run behavior.

## R3a implementation checkpoint (inert, not a supported remote capability)

- `backend/internal/sandbox/infrastructure/sshremote/jobs_v2_gate.go` now defines a backend-owned v2 manifest/attempt preparation value and a fail-closed Linux execution-point script generator. Its order is: per-TaskID exclusive `flock` → check resolution/start markers → exclusive start-intent create → `sync -f` of marker and directory → user shell. A later invocation with the same marker will not re-execute command side effects.
- `jobs_v2_gate_test.go` verifies bounded input/identity handling, script digest/attempt separation and launch ordering. It also contains **Linux-only** dynamic gate tests against disposable local filesystem commands; macOS skips them because util-linux `flock` is unavailable.
- The production `Jobs.Submit` path **still writes legacy v1**. This checkpoint deliberately does not implement SFTP durable manifest/launch-intent persistence, remote capability probing, owner Resolve, proof-bearing resolution markers or a lifecycle QuiescenceProof. Therefore it is **not R3a closure and must not be advertised as `job.recover` or safe to run**.

## Delivery boundary

Implement R3 in the SSH Backend in two independent, verifiable steps:

- **R3a**: versioned durable journal + execution-point gate + negative-path tests. Keep any unverified v2 capability unavailable to Profiles/Marketplace; do not switch existing production v1 submissions.
- **R3b**: Core Recovery fence and explicit owner-only `Resolve` wired to plugin evidence and lifecycle proof, with concurrent late-writer fault tests and operator-approved real validation.

Only after both gates pass may the optional `job.recover` capability be advertised. Do not add Core RemoteTask, a global Task table, Scheduler or Agent loop.

## References

- https://man7.org/linux/man-pages/man1/flock.1.html
- https://github.com/openssh/openssh-portable/blob/master/PROTOCOL
- https://man7.org/linux/man-pages/man1/systemd-run.1.html
- https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html
- https://www.sqlite.org/lang_transaction.html
