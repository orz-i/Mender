# Agent 原生执行与任务审计证据边界

Status: scope approved for implementation (2026-10-18); baseline source and production Agent history examined. This is one bounded handoff, not a new permanent phase ladder. Owner: Mender remains a tool/resource/governance supplier, never an Agent Loop.

## Baseline, observed without modifying production state

- Owner rebuilt the latest Mender Wails; an actual Grok grok-4.7/xhigh no-tool prompt Task completed. This proves one live prompt, **not** any native-tool permission coverage or the bounded pre-prompt recovery branch.
- Read-only production SQLite at handoff has Agent schema v12 / Policy v3, three existing Host Agents (Grok/OpenCode/Codex), zero governed placements, 49 historical Tasks and 184 ACP approval rows. Legacy completed/failed outcomes are mixed; historical failures arose under differing binaries, settings and prompts, so they are NOT an unbiased cross-version success rate.
- OpenCode real coding attempts atask_d74b13efd49b5aae3aa1588d6da183bf and atask_32157f014b3a6b10186a00f9077f14cb both finished failed with the *safe* persisted category "Agent rejected the ACP prompt" and no successful delegated edit. This is evidence of rejection, **not** evidence of its upstream root cause. Other real OpenCode tasks have completed code changes.
- Existing ACP v1 `session/request_permission` is Agent-initiated cooperative owner review. The Host CLI may use native file/shell/network behavior without sending an ACP approval request; no ACP approval row cannot prove no external side effect. Actual mandatory protection comes from verified process isolation and canonical Mender Tool execution. Production Host Agent configuration remains unchanged.
- OpenCode v2 ordered permission rules also note that shell runs with host-user authority and directory detection is best effort. Its `execute:ask` in normal Host mode permits cooperative owner review, not strict containment. The existing governed-only `restricted_host` ToolSupply route remains the verified safe execution option on this macOS host; do not silently install or change it for an existing user Agent.

## Bounded engineering work

1. **Truthful owner task result feedback**. In the existing Agent Task *detail dialog only*, show a compact, non-actionable note for `agent.isolation === "host"` that ACP approval history is not an exhaustive native execution log and an empty approval history does not prove no reads/writes/commands occurred. Do not spread explanatory paragraphs across overview/list views. Do not misstate restricted_host as universal network/VM containment.
2. **Focused React tests and bilingual i18n**. Cover a completed Host Task with zero approvals, a failed Host Task, and a restricted_host Task without the Host-only note; use only fixed local translation keys. No backend contract or Task history changes.
3. **Real OpenCode coding ownership**. Delegate source edits/tests for the three nonoverlapping files `packages/app/src/domains/agents/agent-detail-page.tsx`, `packages/app/src/domains/agents/agents.test.tsx`, `packages/app/src/i18n/zh-cn.ts`. Agent must genuinely edit source and run focused Vitest, not just read. It must not commit, alter any unrelated file, change authorization/isolation, run destructive commands, access credentials or restart Wails/Tunnel. Coordinator reviews actual diff and independently tests/commits source. Record FAILED honestly if Agent does not deliver.
4. **Failure stage investigation is separate**. Existing `prompt_rejected` says only that an ACP JSON-RPC error was returned. Do not invent a cause or expose upstream error messages, stderr, paths, bearer tokens or prompt payloads. Add backend code only for a demonstrable, bounded diagnostic gap with an independent regression; otherwise document evidence and defer. No unbounded retries or hidden prompt replays.

## Evidence gates and rollback

- Run targeted React tests, i18n coverage, strict TypeScript and full `pnpm check`; native Wails Go build only if needed for an owner GUI acceptance. Any final source change is a separate, reviewed Git commit after this document.
- Verify production SQLite read-only and preserve every existing Agent, approval policy, grant, workspace, Tool placement and historical Task. This round makes no user grants, does not change Grok/OpenCode defaults and does not restart the owner's Desktop/Tunnel.
- Untracked `.agents/`, `.pi/`, `.intent-loop/` and `.commandcode/` remain untouched. No branch push.
- No claim of complete native filesystem/shell audit from either ACP approvals or a successful Agent response. This is a UX evidence-boundary correction, not a security sandbox implementation.

## Actual OpenCode delivery and coordinator boundary (2026-10-18)

- Submitted a **real code-writing** OpenCode Mender Task `atask_f9ead25cd8d85be36b63f63f806afb52` through Profile `agent.delegate`. It was explicitly scoped to three React/i18n source files, direct edits and focused Vitest/typecheck, not a read-only review. The Task terminal state was **failed**, persisted safe diagnostic **`Agent permission request was rejected`**, with 3 `read / allowed_once` approval records and **zero tracked source edits or passing Agent-authored tests**. It is NOT an Agent-delivered code contribution; no repeat delegation or approval broadening was attempted.
- The coordinator then independently implemented exactly the scoped UI/i18n/test change, ran canonical root Vitest (**19 passing tests**), strict `@mender/app` typecheck, Chinese i18n coverage (**1334/1334**) and `git diff --check`; source commit `4246c85`. The Task detail modal now discloses native audit coverage only for Host Agents (completed/failed), not as an always-visible overview description. Four focused new assertions cover Host completed/failed, restricted_host omission and Chinese output. Owner's runtime and persistent configuration were not changed.

## Bounded permission failure classification amendment (docs before code)

The live failed task above exposed a diagnostic blind spot: the existing `permission` stage covers several **different Mender boundary outcomes**. Current ACP parsing either rejects an invalid/missing permission request before it is saved, rejects an unsupported Agent→Client method, or refuses an otherwise well-formed request through current owner/Policy authority. A failed Task with no new approval row is not enough to infer which happened. Do NOT treat an absent row as evidence that a native operation occurred or did not occur.

A limited diagnostic-only amendment may distinguish two **trusted local code-path causes** from the existing general `permission` result:

1. Invalid/incomplete `session/request_permission` payload rejected by `parseACPApproval` ⇒ fixed `permission_request_invalid`, result text `ACP permission request is invalid`. Do not store raw params, suspicious option IDs or tool descriptions.
2. Any other unadvertised Agent→Client request handled by fail-closed `respondToAgent` (such as `fs/read_text_file`) ⇒ fixed `permission_client_capability_unavailable`, result text `Agent requested an unsupported ACP client capability`. Do not interpolate method name or payload.
3. Existing admitted but rejected/expired/policy-denied approvals, replay/budget limits and a no-requester state ⇒ unchanged `permission`, result text `Agent permission request was rejected`. No new allowed option, retry or auto-grant.

Both new codes must come from local typed sentinels, not Agent-supplied diagnostic strings. Tag only terminal prompt/setup errors, preserve `errors.Is` for the existing denied sentinel, prefer caller cancellation/deadline, and keep the diagnostic allowlist strict in the Application. Add synthetic ACP subprocess fixtures covering both real invalid/unsupported Client call paths, fixed-result tests and redaction tests, bilingual owner translations and React diagnostic rendering. Never persist raw RPC, stderr, paths, prompts or credentials. This is a **classification aid**, not an assertion of the upstream OpenCode's exact failure cause; real runtime testing needs an owner-controlled rebuild.

## Acceptance and rollback

- Unit/integration, Go Race Detector, lint, i18n/React/type checks and full `pnpm check` must pass. Original failure class remains stable for normal owner rejection. Source changes are independently reviewed/committed in a separate code commit after this doc amendment. No database/schema or Tool API changes.
- Task history, live Host trust, Sandbox, Agent default settings, source directories and native process stay unchanged. Do not rerun failing real development repeatedly until evidence can distinguish the boundary; existing `atask_f9ead25cd8d85be36b63f63f806afb52` is immutable history, not retroactively reclassified.
