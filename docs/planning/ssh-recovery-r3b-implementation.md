# R3b implementation contract — bounded v2 SSH recovery

- **Status:** Implementation closed for local verification (2026-10-13); operator-approved Rocky Linux/systemd real-host acceptance remains pending. Not a production-readiness declaration.
- **Project boundary:** Mender remains an LLM/Agent Tool and Resource aggregation, governance and supply platform. No core task database, scheduler, agent loop, or generic RemoteTask.
- **Supported rollback:** Existing v1 TaskIDs, ToolIDs, grants and lifecycle remain unchanged. New v2 is explicit opt-in and must fail closed if remote Linux/fsync/flock/atomic-rename prerequisites fail.
- **Safe unit:** A v2 record shares the existing per-Sandbox TaskID namespace and dedup tombstones; the manifest selects v1/v2. There is no silent v1 fallback on a v2 attempt and no automatic retry for an uncertain execution.
- **Evidence:** SSH plugin owns immutable per-task v2 manifest, gate script, launch-intent, start-intent, exit and resolution, and produces a versioned evidence digest with Activity distinct from execution outcome.
- **Recovery:** Owner-only, explicit, paused admissions, exact generation/owner/Policy, Core exclusive short recovery lease, evidence-digest optimistic check, then backend under the same remote `flock` used by the script: revalidate script+manifest+launch, reject start/exit/partial files, append and fsync immutable operational resolution. Only verified resolution can inform a fresh Quiescent proof. Loss of ACK remains unknown; inspect again, never auto-retry.
- **Failure posture:** No TTL release, no v1 adjudication, no intervention in real existing remote records. Distinct Linux test-owned dynamic gate tests and fake SFTP fault injection required, then separate operator-approved real Rocky/systemd 252 acceptance before claiming production reliability.
- **Owner views:** Resolution is not an MCP Tool or Agent/LLM operation. UI reveals review/resolve only for verified recoverable v2 candidates, with reason and explicit confirmation; no raw shell text, logs, private path, credential, or arbitrary TaskID command.
- **Test criteria:** No double execution across disconnect/ACK loss, no post-resolution user command even if late systemd-run, recovery vs gate serialized, rejected corrupt/symlink/unknown/v1/active cases, paused concurrent submit denied, reconciliation vs stop/destroy CAS, audit of successful resolution, full `pnpm check` and Linux-specific dynamic tests.

## R3b source implementation and operator handoff

R3b implementation commits:

- `a1ed873`: opt-in SSH v2 durable SFTP journal and execution-point lock adapter. The SFTP transport must advertise `fsync@openssh.com`, the pinned Linux host must support verified util-linux `flock`, GNU `sync -f`/`mv`, and a reviewed local filesystem (xfs, ext2/ext3, btrfs). The v2 job uses the **same TaskID reservation namespace** as v1, with no retry of uncertain creates.
- `c7a4c27`: Core-only recovery CAS with persisted paused admission, exact Run generation and local owner/Policy, plus a minimized decision audit (task ID, evidence and reason digests; never raw command). No Core task result table or model-facing recover Tool.
- `19c518b`: explicit owner HTTP/CLI/Wails/SDK recovery APIs, typed OpenAPI contracts, a separate review/confirm UI, and Linux execution-point lock race regression tests.
- Closure hardening also verifies that the signed operational resolution references the exact previously inspected v2 evidence digest; a syntactically valid but mismatched resolution marker cannot release Quiescence. Linux tests use standard-library flock constants to preserve Go module isolation.

The opt-in switch is `MENDER_SSH_JOBS_V2=enabled` **on the trusted Mender host Runtime environment**. Default is legacy v1. A v2 attempt that lacks fsync, verified Linux host prerequisites or an exact binding fails closed; it must not fall back to v1. Do not enable against real user hosts without operator-approved target verification.

A safe owner workflow for **new v2 candidates only**:

1. Pause future submissions: `mender sandbox job-admission <sandbox-id> --pause`.
2. Inspect immutable evidence: `mender sandbox job-recovery <sandbox-id>` (use JSON output to obtain exact fingerprint and generation).
3. If and only if the record reports `prelaunch_candidate` with `recoverable=true`, review the host and evidence, then explicitly request `mender sandbox job-resolve <sandbox-id> <task-id> --generation <observed-generation> --fingerprint <64-char-digest> --reason '<reviewed reason>' --confirm`.
4. Observe the fresh record. A verified `resolved_before_launch` changes **activity proof only**, not the shell exit (`JobState` remains `unknown`); a lost acknowledgment, filesystem mismatch, stale writer, rejected lock or vanished host remains unknown.
5. Retire Tool Suite / Stop / Destroy only when independently authorized and the backend returns fresh quiescent evidence under its lifecycle fence. The permanent Job obligation and TaskID tombstones remain.

### Acceptance evidence and limits

- Real Linux util-linux `flock` + GNU sync/mv shell gate and resolver exercised inside a disposable, network-disabled local Linux container: resolve-before-late-launch, launch-before-resolve, occupied lock, changed Launch Intent and duplicate TaskID execution. No real systemd/SSH host or credentials were used.
- Local Go in-process SSH/SFTP fixture fault injection: lost `systemd-run` ACK, fsync unavailable/failure, wrong fingerprint, corrupt/symlink/partial records, restart/reconnect, v1 refusal, and operational quiescence.
- Core SQLite/HTTP/desktop tests: authorization, pause/resume, cross-Runtime lease conflict, Run CAS, audit hashes, malformed request rejection and explicit confirmation.
- Real Rocky Linux/systemd 252 acceptance is **not established**. This is an operator-owned validation gate, not an implicit approval to connect to an existing remote host. Do not claim supported production recovery until verified against a disposable pinned SSH target.
- The fixed 64-record tombstone quota remains separately tracked; R3 does not add retention or automatic re-execution. Legacy v1 incomplete TaskIDs remain unresolved and continue to fence destructive lifecycle.

### Post-ACK durability proof hardening

A correctly formatted `resolution.json` is **not by itself durable execution-fence proof**. If an SSH response is lost after the rename but before the directory fsync acknowledgement, that JSON can be visible in page cache yet disappear after a host crash. R3b therefore requires a fresh, positive backend-owned `ProveV2ResolutionDurable` check before reporting `quiescent_proved` or permitting Stop/Destroy:

- Verify the **exact per-task** filesystem and Linux host capabilities (not just the shared storage parent), then verify the immutable resolution marker digest under the **same nonblocking `launch.lock` flock** as the execution gate.
- Do not create another TaskID, marker or lock; fsync the **existing marker and its directory**, compare the digest again and re-read the bounded journal. An fsync error, lock contention, digest drift, unsupported filesystem or lost response results in `unknown`/`uncertain`, not release.
- The owner Inspect operation remains *logically read-only*: it never changes task outcome, journal records, scripts, grants or TaskID. It may request safe durability verification of existing records; this can flush pending filesystem metadata but cannot create or rewrite a task record.
- A lost original resolution ACK can be recovered by another positive durability inspection without replaying `Resolve` or the user's shell command. An unresolved v1 record remains protected exactly as before.

Local Go/SFTP fixture tests cover loss and restoration of the proof, and disposable Linux (`flock` and GNU sync) tests cover the fresh marker+directory flush, held-lock refusal and wrong-digest denial. **No live remote SSH or systemd experiment was made**. The operator-approved throwaway Rocky acceptance gate remains mandatory before declaring real-host production readiness.

### Audit ordering closure

Before contacting the SSH Backend for a recovery mutation, Core now commits a minimal **unconfirmed (`unknown`) owner intent** under its exact exclusive recovery lease. The pending event carries the reviewed TaskID, generation, evidence digest, principal and reason digest but no command, secret or free-text reason. If this local write fails, no remote Credential or SSH connection is attempted. Only after the remote evidence-bound resolution returns does Core append a `resolved` or `conflict` confirmation. A lost remote response retains the original unknown intent, and a failed final audit prevents a claimed success without erasing that durable owner trail. Task execution state stays exclusively with the SSH Backend. This uses the existing v16 audit event schema; no Core RemoteTask or new migration is required.
