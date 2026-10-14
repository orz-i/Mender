# SSH Durable Jobs — per-Sandbox permanent v2 activation

- Status: implementation contract, 2026-10-14.
- Decision: one-way owner-authorized v2 upgrade for an exact built-in SSH Sandbox, recorded in the shared Runtime SQLite. No process environment flag in the production Submit path. Once committed, all Runtime processes (Wails, CLI, Tool/MCP) must submit exclusively through v2 for that Sandbox.
- Scope: Core owns the Sandbox binding and owner-only admission/protocol selection; SSH Backend owns v1/v2 journal, remote fsync/flock capabilities, TaskID, execution-point fence and recovery. No Core RemoteTask or scheduler.
- Reuse existing TaskID namespace; a previously reserved v1 key cannot be silently replayed under v2 and is treated as conflict/unknown. Legacy v1 observation, status and recovery remain intact.
- Activation authority: exact SandboxID, owner local Principal, Catalog scope, `sandbox.reconcile` Policy/approval/audit; Run=Ready with exact generation; **pause new submissions** first, verify pinned SSH host and actual v2 SFTP/fsync/util-linux/GNU/local-fs capabilities, then commit one immutable protocol=2 row and audit event under SQLite CAS requiring pause and zero active leases. No auto-downgrade or runtime fallback.
- Every submission re-reads the protocol and reserves its execution lease with the **expected protocol checked in the same SQLite INSERT transaction**; this blocks an old in-flight v1 writer that was suspended before its lease and resumes after activation.
- Failed remote capability check leaves protocol=v1 and Sandbox paused until owner explicitly resumes; no task ID or remote command is created. Activation never overwrites remote files or changes user grants.
- Administrative interfaces: CLI owner `sandbox job-protocol <id>` and `--enable-v2 --confirm`, GET / POST owner management HTTP (no Profile Tool), Wails read-only protocol status/owner activation; existing admission UI shows protocol.
- Post activation: owner resumes submissions. Wails's refreshed Runtime and any later instance observe the same committed protocol without relying on an exported shell variable. Any wrong-host/missing fsync/flock becomes unavailable, no v1 fallback.
- Tests: migration across legacy DB, paused/owner/Run/lease failures, two-SQL-connection race vs Submit, same key v1 remains conflict, local SSH unsupported host unchanged, status across process restart, contract + UI tests, real Rocky capability preflight and a uniquely keyed Profile canary, `pnpm check`.
- Real network must not be changed until local migration and runtime gates pass. Do not clear the existing v1 `unknown` task `sjob_29deeed6015ee33f63cd35f5a7038e1d`; no remote cancellation/deletion.

## Implemented release behavior

- SQLite schema v17 adds an immutable one-way `sandbox_ssh_job_protocols` row and a non-secret protocol activation event. Activation is bound to exactly one built-in SSH Sandbox, owner, Run generation and current paused submissions. The protocol cannot be reset or deleted through ordinary SQLite writes.
- Core `SubmitSSHJob` no longer reads `MENDER_SSH_JOBS_V2`. The shared database determines legacy v1 (unactivated) or durable v2 (activated). The exact protocol is revalidated atomically with the submission execution lease; old writers racing an activation cannot submit v1 after activation.
- An already reserved v1 TaskID remains v1 for status/recovery. Its key is never re-executed by v2; attempting to submit the same key after activation is an explicit conflict. No retroactive rewrite, migration or cleanup.
- An owner must pause future submissions before activating v2. The backend probes the real pinned SSH host, OpenSSH SFTP fsync, util-linux flock, GNU sync/mv and supported filesystem before any protocol mutation. Capability failures leave the Sandbox paused and on v1; they do not create a TaskID.
- Owner CLI: `mender --db "<same Wails database>" sandbox job-admission <id> --pause`, then `mender ... sandbox job-protocol <id> --activate-v2 --confirm`, then `mender ... sandbox job-admission <id> --resume`. A read-only `job-protocol` invocation confirms permanent `durable_v2` status.
- HTTP POST `/api/v1/sandboxes/{sandboxId}/jobs/protocol/activate-v2` requires `{"confirm":true}`; Wails UI makes this available only after an explicit Pause, with a second confirmation and no broad form. Profile Tool/MCP cannot activate v2, but all Profile submissions honor the shared protocol once activated.
- The activation flag is durable and shared by CLI and Wails across restarts (including Mac restarts). No launchctl environment variable, shell profile entry, global default override or Agent loop is introduced.
- The original Rocky v1 uncertain TaskID `sjob_29deeed6015ee33f63cd35f5a7038e1d` retains `manifest_only/unknown/uncertain` and must never be retried or resolved as a side effect of activation.

## Acceptance matrix

- SQLite two-connection and restart tests for owner/Run/paused/lease protection, immutable one-way update, protocol-specific submission CAS and v1 refusal, plus legacy database migrations and audit tests.
- Local in-process SSH fixture must reject activation if fsync/flock prerequisites are unavailable, leave no new TaskID and retain Pause after restart.
- Owner HTTP rejects malformed body and unconfirmed operation. UI requires explicit Pause, explicit confirmation and maintains pause state after activation.
- Rocky v2 enablement is to be rolled out only after `pnpm check`, local app rebuild and owner-selected database backup. Then reopen Wails and verify a uniquely keyed Profile v2 canary.
- The older `liveacceptance` smoke harness now activates v2 on its **new test-owned Sandbox** before first submit; the prelaunch fault injector refuses Sandboxes lacking the durable v2 activation row. It never touches the existing user-owned Rocky TaskID.

## Actual Rocky rollout — 2026-10-14

- Full `pnpm check` passed (`exit=0`); Go race, desktop bindings, SDK, UI and build included. Separate live-tag lint passed.
- Backed up the live Wails SQLite consistently to `~/Library/Application Support/Mender/backups/mender-pre-ssh-v2-20261014-151308.db`, integrity `ok`, mode `0600`. Backup is local and must not be published.
- With the trusted owner CLI and the existing Rocky Sandbox `sbx_1b80d48ebb2591ecae6df534d9721e63`, paused admissions, verified real remote fsync/flock, and committed the irreversible v2 activation event in schema v17. No old SSH task was modified.
- Restarted the Wails v3 dev process with freshly rebuilt Go/TypeScript source. New Wails process launched successfully, reading the same shared SQLite. Resumed admissions; final SQL verification: `protocol=2`, `paused=0`, schema `17`, Run still Ready.
- Invoked the unchanged `default` Profile's canonical SSH job submit Tool using the updated owner CLI, with a new random idempotency key and a harmless fixed-output command. New TaskID `sjob_35b4cd43e820de0109fea3cc69215b31` was accepted (`running`), then via the same Profile result Tool returned `completed`, `exitCode=0`. This confirms the ordinary governed Profile -> Core -> v2 SSH Backend path. It is NOT a claim that the ChatGPT cloud MCP tunnel was available during this verification.
- Existing v1 Job history stays under the same Sandbox. The previous unknown TaskID must remain unknown and cannot be auto-upgraded/replayed. This production setting survives process/system restart without exported environment flags.
- **Separate connectivity note:** the Wails dev restart left the previous OpenAI Tunnel not running; it must be explicitly re-established through the normal managed MCP server/Tunnel workflow. Do not infer cloud ChatGPT connectivity solely from the Profile CLI canary. No replacement Tunnel was provisioned.
