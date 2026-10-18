# ADR 0050 — Antigravity CLI as a native headless Agent backend

- Status: accepted for implementation (2026-10-18)
- Scope: built-in Agent Backend, not a Provider, ACP driver, or Mender Agent Loop.

## Decision

Offer the installed Google Antigravity CLI (`agy`) as a fourth, host-reviewed built-in Agent Backend. Use its **official** non-interactive `--input-format stream-json --output-format stream-json` protocol, with prompts on standard input and `result` events on standard output. Reuse the existing local OS identity and Antigravity's keyring/session. Never inspect, copy, reauthenticate, or persist its credentials.

The established Agent/Task ownership, Profile grants, workspace binding, admission audit, task cancellation and output limits remain unchanged. Route this backend to a narrow native headless Executor; the other three Agent backends continue through their existing ACP v1 Executor. This is not an ACP adapter: no `session/request_permission` callback exists in the Antigravity headless protocol. No Mender ACP Approval is synthesized. Native file/command/MCP permissions follow the user's Antigravity settings. The Mender UI must say so. The owner may **explicitly** select an Agent/workspace-scoped, expiring **24-hour trusted** mode to launch new headless Tasks with the CLI's `--dangerously-skip-permissions` (YOLO) option. This approves all of Antigravity's native tools without Mender ACP approval events; the `--sandbox` flag still applies terminal restrictions. The option is absent in default/manual mode, in model inspection, after expiry or revocation, and for another workspace. A Profile/model/Task cannot enable it. This authorization snapshot is checked before each process launch; revoking does not stop already-running processes. `workspace_read` cannot be represented faithfully in AGY and is rejected. Never claim that absence of Mender approval receipts means Antigravity did not use native tools.

Use the existing trusted executable allowlist (`~/.local/bin/agy` etc), canonical workspace resolution, process isolation and fixed environment. Enforce output/frame bounds, strict structured terminal result, cancellation/timeout, safe persisted diagnostics (no raw stderr, prompts, account names, native tool input/output or bearer tokens). Do not put the prompt in command-line arguments. Model flags must be passed as literal argv entries only after validation. Explicit model lists are owner-triggered (`agy models`, no model prompt) and validated, not synthesized. No automatic installs or system settings mutation.

Because the Antigravity headless interface has no Task-scoped governed MCP Tool injection or ACP request/response bridge, reject a nonempty Mender ToolSupply **before** starting `agy`. This feature does not add a second tool path or privilege elevation. Host mode uses the OS user's privileges; restricted_host may limit or break keychain access and must never fall back automatically. Antigravity's own `--sandbox` mode is an extra defense-in-depth for terminal tools, not a replacement for Mender isolation or permission policy.

## Acceptance

1. Built-in marketplace card → Agent creation → Profile grants → delegate/status/result, with no new Provider type; existing backends unaffected.
2. CLI availability is a filesystem-only probe; `agy` login is reused, not managed; model inspection never sends a prompt.
3. Explicit default/task model and optional effort map to `agy` flags; invalid and unsuccessful terminal results fail closed, not as successful Task output.
4. New Agent schema v13 extends the CHECK constraint without changing historic Agents/Tasks/permissions. Migration uses pinned connection, FK pre/post checks, trigger preservation and re-enable verification.
5. Tests exercise stream events, malformed/missing result, oversized output, cancellation, absent executable and governed-tool refusal. Run Go/SDK/frontend checks and a bounded real Antigravity no-tool smoke if the local CLI works.

## Not implemented

Native ACP approvals, live streaming UI, multi-turn conversation continuity across Tasks, credential management, background scheduling, automatic installation, or arbitrary execution args from Task prompts.

## References

- https://www.antigravity.google/docs/cli/headless/
- https://www.antigravity.google/docs/permissions
- `agy --help` / `agy models` on macOS, CLI v1.4.2 (2026-10-18)
- Owner clarification (2026-10-18): explicitly permit AGY YOLO through `--dangerously-skip-permissions`, without granting it implicitly to all Tasks.
