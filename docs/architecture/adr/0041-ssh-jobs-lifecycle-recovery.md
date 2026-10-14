# ADR-0041: SSH Jobs lifecycle invariants and evidence-bound recovery

- Status: Accepted for bounded recovery implementation
- Date: 2026-10-13
- Refines: ADR-0035, ADR-0039, ADR-0040
- Goal: keep Mender a **Tool/Resource aggregation, governance and supply platform** for external LLMs/Agents. SSH detached tasks and their execution state belong to the SSH Sandbox backend.

## Audited baseline

- `sandbox/infrastructure/sqlite/lifecycle.go` checks Workspace Provider references at Destroy CAS, but not `sandbox_job_suite_bindings` / Jobs Provider references. Run state (not Sandbox row deletion) bypasses the foreign key.
- `bootstrap/sandbox_job_tool_suites.go` creates a Provider, synchronizes Tools, then claims a Suite in three persistent steps; a lone unclaimed Provider currently appears as conflict instead of recoverable incomplete.
- SSH Jobs v1 writes a private directory, manifest, script, then calls `systemd-run`; interrupted or lost-ACK execution remains `unknown`. A real Rocky timeout left a manifest-only record; **absence of the script now is not proof of non-execution**.
- Core execution leases have no TTL. This is intentional: a stale SQLite lease cannot prove a remote writer stopped.
- Current SSH Job status, logs and cancellation are plugin-owned; ordinary MCP Tools remain separate governed functions. No global Mender Task store or MCP Tasks emulation is introduced.

## Decisions / invariants

1. **Destroy reference closure.** The final Run-generation CAS must reject every live Workspace or Jobs Provider/Suite link for the exact Sandbox, in the same SQLite transaction as the state transition. A managed obligation alone is not a Provider reference; remote quiescence is separately required. No delete cascade.
2. **Inspections are inert and owner-only.** The SSH plugin may return bounded TaskID, evidence class, phase, evidence digest, and activity verdict. No commands, secret paths, credential payloads, raw logs, mutable repair actions, or network mutations are exposed. A local owner may request inspection after independent Policy admission. Profile-facing `jobs.status/logs/result` remain unchanged. Absent/unreachable host yields explicit unknown, never success.
3. **Admission pause is distinct from Stop.** A persisted, owner-controlled `job_submissions_paused` flag prevents `ReserveSSHJobSubmit` in the same atomic SQLite operation, across Runtimes. Pause permits existing observation and authorized cancellation; normal Stop/Destroy still require plugin-proven quiescence. Resume checks exact ownership and run generation, and does not automatically retry unknown tasks. Pause and resume produce audit evidence.
4. **Remote recovery needs execution-point fencing.** A v2 task must write an immutable, verified reservation and use a remote launch gate that cannot be bypassed by a late writer. A recovery resolution must be made while holding a verified exclusive remote launch fence and must remain effective for delayed `systemd-run` launches. It never forges an exit code or drops an idempotency tombstone. A missing `systemctl` Unit, old file absence, timestamps, boot ID or expired local lease alone is **not** sufficient evidence. v1 tasks are inspection-only until an explicitly reviewed legacy process can prove non-execution.
5. **Actions are separate.** `inspect` only reads; `pause` only blocks new submits; `resolve` requires a targeted TaskID, expected immutable evidence fingerprint, explicit disposition and reason, newly checked owner/Policy, exact Run/binding, Core short exclusive recovery fence and a backend proof. Failed recovery retains unknown and the lifecycle obligation. There is never an automatic replay, force clear, time-based lease expiry or transparent fallback to host execution.
6. **Do not conflate task status with safety.** Execution outcome (`completed/failed/cancelled/unknown`), activity proof (`active/quiescent_proved/uncertain`), and Sandbox lifecycle (`ready/paused/stopped/destroyed`) are independent. Every error path defaults to denying destructive lifecycle.
7. **Compatibility and scope.** Keep canonical ProviderID/ToolID, Tool Suite SourceKeys, Profile grants, SSH trust/credential handling and audit semantics. No shared RemoteTask, Job Scheduler, Agent loop, changing Protocol v1 or MCP Tasks dependency. Never mutate real remote or old unknown TaskID during test/acceptance.

## Work packages and gates

- **R0:** atomic Destroy reference guards (Workspace + Jobs, provider configurations and host-managed Suite links), same-DB concurrent transactions and regression fixtures. Commit separately.
- **R1:** plugin-owned read-only evidence inspector and bounded owner-level API; test complete, partial, inaccessible, terminal, active, fake/symlinked and stale evidence cases with test-owned SFTP only.
- **R2:** persistent paused admission flag with atomic submit/owner operations, audit and CLI/HTTP/Desktop projection; cross-Runtime submit/pause/stop/cancel tests.
- **R3:** v2 execution-point remote fence and safe explicit prelaunch recovery for *new v2 tasks only*, guarded by fresh evidence digest + host fencing and remote immutable resolution. Legacy v1 remains unknown and blocked. Fault injection at every write/ACK boundary.
- **R-close:** `pnpm check`, Go race, tests, desktop bindings, security checks and clean tracked tree; local in-process SSH fixtures only. Genuine Rocky remediation requires separate operator authorization.

## Operational posture

A blocked Sandbox is not an error to be overwritten. If the host is unavailable, a v1 job is incomplete, or a remote writer cannot be excluded, operator may pause new submissions and inspect, but normal Stop/Destroy must remain blocked. Do not claim a fully recoverable Task until remote fencing and crash tests provide proof.
