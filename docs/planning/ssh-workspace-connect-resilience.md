# SSH Workspace connection resilience and truthful Tool errors (2026-10-13)

Status: **Closed (2026-10-13)**; document-first focused follow-up.

## Evidence
- Interactive Mac SSH with T.pem authenticated as root and verified /data/workspace, but required ~11.5 seconds amid intermittent TCP timeouts.
- Mender currently limits SSH connect to 12 seconds overall, TCP dial to 8 seconds, and the SSH/SFTP handshake to 10 seconds; this may reject real reachable hosts.
- Rocky's pinned host fingerprint matches the Mac OpenSSH record. Rocky's own stored SSH login private-key identity has not been compared with T.pem. No private-key bytes may be emitted.
- Rocky Sandbox status is persisted ready, its workspace Tool Suite is available, and default Profile capability grants are present. However Rocky ToolIDs are not in the default Profile's Tool selection. The currently visible similarly named Workspace tools belong to Mender Provider bound to Alpine Docker.
- SSH typed connection failures currently collapse to provider_error / provider execution failed and lose actionable stage metadata.

## Contract
1. Increase SSH connect budget to 45 seconds end-to-end and per-TCP-attempt timeout to 20 seconds. Retry only transient network dial errors, at most twice, with a small bounded backoff; preserve parent cancellation. Never retry authenticated operations, host-key mismatches, password/public-key auth rejection, workspace edits or shell commands. Retain host-key pinning, known_hosts verification and RSA-SHA2 signatures.
2. Surface ONLY allowlisted, safe SSH ConnectionFailure codes in Tool failure messages and audit summaries. Do not reflect arbitrary provider, network, SSH server or filesystem messages. Preserve existing Tool failure schemas and Provider error classification, with stable 'ssh_remote_failure:<code>' markers.
3. Test network dial timeouts/retry, exhaustion/cancellation, no handshake retry, slow SSH handshake within new budget, credential-invalid/no retry, native end-to-end SSH/SFTP/workspace; regression that Docker/other Provider failures remain redacted. No user-owned remote host calls in automated tests.
4. Re-check Rocky Tool Suite and Profile exposure. Adding remote root read/edit/shell tools to default Profile changes the reach of active ChatGPT tools, and must not silently replace existing Alpine tool IDs or restart current MCP Server/Tunnel. Operator-controlled Profile change may be staged independently after safe verification; existing grants alone do not expose Tool IDs.
5. Document first; phased commits; full pnpm check and Wails Go build. Do not stage .agents, .pi or .intent-loop, do not copy private keys, and do not restart active services without explicit request.

## Acceptance and rollout (2026-10-13)

**Status: Closed.** Document-first and segmented Go commits:

- `d700baf`: documented operator evidence, intended behavior and security boundaries.
- `cd38285`: SSH total connect budget 45s; two TCP dial attempts of up to 20s each with a 300ms bounded delay only after transient dial timeouts. Parent cancellation is respected; never retry host key verification, authentication, SFTP, workspace or Tool execution.
- `de29c41`: preserve only allowlisted SSH stage diagnostics through the existing `DiagnosticSummary` contract into both Tool failures and audit. For SSH Workspace Tools only, select a 60s implicit execution timeout, still capped by Runtime Budget and caller context; all other Providers retain the existing defaults. The same transport budgets apply to first-use host inspection.
- `7439cb0`: test-owned local SSH/SFTP server deliberately delayed pre-handshake by 13 seconds; the revised deadline succeeds, whereas the former 12s overall budget could not.

### Validation
- Retry success/exhaustion, immediate refusal, parent cancellation, never retry known-host mismatch/public-key authentication/workspace failures, first-use host trust, RSA-SHA2 and SSH/SFTP end-to-end tests all pass.
- `TestSSHWorkspaceBudgetAndTypedToolFailureAreScoped` verifies the SSH-only 60s implicit budget, preservation of an explicit shorter deadline, unchanged local Workspace behavior, and safe `ssh_remote_failure:credential_invalid` Tool response and audit.
- `pnpm check` final run returned **exit 0**: Go lint/test/race, SDK/protocol/desktop binding checks, TypeScript, ESLint, i18n, **202/202 frontend tests**, Web/Desktop frontend builds. An earlier unrelated Docker UI timer assertion flaked during the first full run; it passed both in isolation and in the final full run. Native `GOWORK=off go build ./...` in `apps/desktop` succeeded (existing macOS deployment-target linker warnings only).
- Real Rocky tests through the owner's Mender CLI using the existing native credential and fixed host-key verification: safe Shell returned `root` and `/data/workspace`; a unique temporary file was created, read, searched, replaced, read again, and deleted successfully. All six steps passed, exit status 0, cleanup confirmed. The successful login proves credential usability, but not that the stored private key bytes are identical to the Mac's `T.pem`; no private keys were inspected or copied.
- Live `default` Profile previously had 12 ToolIDs and preexisting grants for Rocky's read/search/edit/shell. Explicitly added its four *existing* ToolIDs with `rocky_read`, `rocky_search`, `rocky_edit`, `rocky_shell` Profile aliases and Rocky-distinguishing titles/descriptions via Mender's normal CLI. All 12 original ToolIDs retained; final count 16. A subsequent read-only Rocky shell Tool call with `--profile default` completed successfully. No implicit Profile grants were created and no Alpine tools were removed.

### Operational boundary
- **No explicit service restart was initiated by the verification workflow.** However, a final process inspection at about 14:46 local Mac time found Wails dev had relaunched automatically (new desktop process) and no active `tunnel-client` child was present. This is typical of a dev watcher rebuild; do not claim the earlier live MCP Server is still online. The next operator action is to start the `default` Profile MCP Server/Tunnel in the currently running Wails app, then refresh the ChatGPT tool catalogue. Confirm actual ChatGPT visibility after reconnect, not from Profile DB membership alone.
- **Privilege note:** This Profile now selects the Rocky Workspace tools, which operate as the configured remote `root` account. Existing Policy/Principal/Lease/grants and fixed host-key verification remain in force; Shell is not OS containment. Avoid automated remote root edits without authorization.
- Private-key material and remote business files were not read. Tests touched only one generated file in the configured remote workspace and removed it. No keyring rotation, pin change, remote package install or **operator-initiated** daemon restart occurred. `.agents/`, `.pi/` and `.intent-loop/` remain untouched and uncommitted.
