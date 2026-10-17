# Phase 44 follow-up — ACP heterogeneous session updates after owner approval

Status: implementation and automated/isolated real ACP acceptance complete; production rerun awaits a new Wails native build. Date: 2026-10-17.

## Evidence

The owner explicitly selected **Allow once** for the OpenCode Task `atask_6a8b9e561b0c5c0fb0f21ceb5ccee80a` to read only `README.md`. Mender's live v10 SQLite recorded Approval `aapr_d7c6054ddb4bade19d00736f1da3dc8a` as `allowed_once` (requested kind `read`, review target `README.md`, `can_allow=1`) and the Task failed immediately afterwards with generic diagnostic `Agent execution failed`. No project files or grants were changed.

A separate short-lived Go test running the **actual Mender ACP Runner and local OpenCode v2.0.30** in a disposable directory reproduced an explicit in-root read approval, followed by the precise underlying error: **`ACP malformed session update`**. An independent isolated ACP v1 client with the same OpenCode overlay, approved on the same disposable file, successfully received `stopReason=end_turn` and the expected file line, without calling extra client filesystem methods or modifying the file.

The Runner currently unmarshals **every** `session/update.update.content` into a fixed `{type:string,text:string}` Go struct. ACP sends tool-call/tool-call-update content with other valid shapes, which cannot be parsed into this text-only type. The failure is therefore in Mender's heterogeneous update handling, not a user rejection or an AgentGrant/Model/ToolID issue.

## Repair contract

1. Parse the outer `sessionId`, `update.sessionUpdate` and a **`json.RawMessage` content**. Accept heterogeneous well-formed updates without treating tool-call content as an Agent text chunk. Decode the typed `content` object **only** when `sessionUpdate=agent_message_chunk`; append only bounded text chunks and enforce the existing `MaxResultBytes` limit. Preserve validation of foreign SessionID and malformed JSON, reject malformed text chunks, never retain full raw tool calls, diff, secrets or thought content.
2. Do not auto-approve or otherwise change the ACP permission dispatcher. Only one reviewed file read may be allowed once by the human local owner; edit/shell remain reject-only. Existing Task/Approval lifecycle, Profile grant, ToolID/ProviderID and SQLite v10 are not modified.
3. Synthetic fixture emits **array and object tool_call/tool_call_update contents before and after an approved read** and confirms task completion, bounded text extraction and no raw storage. Include malformed/foreign/missing-text negative cases, cancellation and no client fs/terminal capability.
4. Real Runner test uses `opencode acp` under the fixed host-owned review policy and a **temporary file** (not the user's project), model `opencode/glm-5.3-flash-free`, effort `max`, simulated allow-once callback, and verifies `end_turn` and expected read result. Remove temporary test files immediately after acceptance.
5. Run `pnpm check`, Go Race Detector and generated Wails/SDK parity. Commit docs then fix/test then acceptance records. Do not restart the user's live Wails/MCP Tunnel without a separate zero-task safety preflight; the owner can retest with a fresh deliberate click after rebuilding.

## Safety/truthful result

The first positive production approval proves the **owner's decision and audit ledger** worked, not that the read completed. No successful read should be claimed until the new Runner is deployed and a new explicit owner approval returns `completed`. A disposable fixture approval never substitutes for the owner's decision on production tasks.

## Implemented fix and acceptance evidence

- **Docs-first:** `d1a25a0` recorded the failed production one-time approval and protocol-parser contract before code changes.
- **Code fix:** `e510a1c` changed the ACP `session/update` envelope to hold `update.content` as `json.RawMessage`. It only interprets a typed text object for `agent_message_chunk`; heterogeneous `tool_call` and `tool_call_update` content arrays remain valid protocol notifications and are discarded without copying untrusted Tool content into Task results.
- **Synthetic tests:** approved ACP fixture now emits both heterogeneous tool-call update types before returning `end_turn`, and the assertion still reads only bounded Agent text. Malformed `agent_message_chunk` content, wrong sessions, client-side file calls, unadvertised choices, cancellation and oversized result continue to fail closed.
- **Real Runner contrast:** an isolated test used Mender's **actual Go ACP Runner**, host-reviewed local OpenCode v2.0.30, model `opencode/glm-5.3-flash-free`, effort `max`, a temporary single-file fixture, and a test-only allow-once callback. Before the fix: exactly one reviewed read request followed by `ACP malformed session update`, no Task result. After the fix: exactly one reviewed read request followed by `end_turn` and result `synthetic local fixture`; no native client filesystem/terminal capabilities were advertised. The fixture test and temporary directory were cleaned immediately. The positive callback here is an isolated **synthetic local test** and does not substitute for the human owner's production approval.
- **Deployment boundary:** the running Wails native binary was compiled before the fix; its existing OpenAI tunnel was left connected and the live v10 database was not changed. A new human-owner approval in the real UI can only be retested after the native Wails rebuild/restart and tunnel reconnection.

## Final regression gate and deployment boundary

- `d1a25a0` is the document-first diagnosis. `e510a1c` fixes the production ACP Runner and adds permanent heterogeneous `tool_call`/`tool_call_update` acceptance tests, plus a malformed Agent text chunk fail-closed test.
- Before fix: real temporary-file OpenCode v2.0.30 Agent + **Mender's Go ACP Runner**, explicit model `opencode/glm-5.3-flash-free`, effort `max` and test-only allow-once callback reproduced `ACP malformed session update` immediately after permission. After fix: the same real Runner returned `synthetic local fixture` with one approved temporary read, no client FS/terminal method, no error; temporary test source/file were removed.
- **Final canonical `pnpm check` exited 0** (`/tmp/mender-phase44-positive-fix-check.exit`, log `/tmp/mender-phase44-positive-fix-check.log`): full Go unit tests and Race Detector, plugin/SDK/protocol checks, SDK/Wails bindings parity, Go lint, strict TS/ESLint, i18n **1263/1263**, Vitest **44 files / 253 tests**, Web/Desktop production builds. An independent native Wails `go build` passed with only nonfatal macOS deployment-target warnings; the temporary binary was removed.
- The actual production Task `atask_6a8b9e561b0c5c0fb0f21ceb5ccee80a` has already persisted `allowed_once` under owner approval `aapr_d7c6054ddb4bade19d00736f1da3dc8a`, but finished `failed` on the *pre-fix Wails process*. **Do not relabel this Task as a successful read.** Revalidation requires a *new* Task, a *new* explicit owner Allow once, and successful OpenCode/Mender `end_turn` result after the native app is rebuilt and the MCP Tunnel reconnected.
- No live user database schema, Profile grant, ToolID, Agent or source file was changed during the repair; the existing Wails native binary and Tunnel remained running until the owner controls the deployment.
