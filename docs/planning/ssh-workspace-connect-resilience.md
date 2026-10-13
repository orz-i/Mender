# SSH Workspace connection resilience and truthful Tool errors (2026-10-13)

Status: In progress; document-first focused follow-up.

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
