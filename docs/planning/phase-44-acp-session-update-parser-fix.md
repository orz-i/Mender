# Phase 44 follow-up — ACP heterogeneous session updates after owner approval

Status: confirmed regression, implementation pending. Date: 2026-10-17.

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
