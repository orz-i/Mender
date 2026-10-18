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

## Acceptance and real delegation receipt

Pending real OpenCode delivery, independent tests and documentation of the observed terminal Task outcome.
