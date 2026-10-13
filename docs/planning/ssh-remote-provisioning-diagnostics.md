# SSH Remote provisioning diagnostics — safe errors end to end (2026-10-13)

Status: **In progress**. Follow-up to Phase 32 and root-login advisory closure; document first, incremental commits, no compatibility bridge.

## Evidence and problem
- The 2026-10-13 desktop screenshot shows a correctly reached SSH Remote review step and a generic Simplified Chinese "Unexpected error" after Create; **the actual remote failure stage is unconfirmed**.
- The SSH dialer erases network/host-key/auth/SFTP/root validation errors to the same `sandboxdomain.ErrBackendUnavailable`. The Sandbox application also replaces the dial error with a generic wrapper. No safe stage/code reaches Wails or HTTP.
- `errorMessage()` deliberately hides unrecognized English Wails messages under zh-CN, turning distinct operational failures into "发生意外错误". The primary user defect is missing actionable, safe diagnostics, not evidence for relaxing root/host identity policy.
- A pinned `ssh-rsa` value may be a server host public key or, mistakenly, a user's login public key. These are different identities; **do not assert a mismatch without a verified handshake**.
- No real remote host/key may be accessed in this work. The user's currently running Wails app, Tunnel and Keychain are not modified.

## Contract
1. Introduce a first-party typed, allowlisted `sshremote.ConnectionFailure` with stable `ssh_remote_failure:<code>` error text containing **only the code**. It must unwrap to `sandboxdomain.ErrBackendUnavailable` so existing callers and HTTP status retain semantics. Do not include a host, username, directory path, public/private key, SSH library text or OS/remote error in any management/Wails/HTTP error. Preserve original failure only in local control flow for safe categorization.
2. Distinguish at least: `credential_unavailable`, `credential_invalid`, `network_timeout`, `network_refused`, `network_unreachable`, `host_key_mismatch`, `authentication_failed`, `ssh_handshake_failed`, `sftp_unavailable`, `workspace_missing`, `workspace_permission_denied`, `workspace_invalid` and a generic `ssh_remote_unavailable`. Classify port refusal/timeouts with `errors.Is` and typed net errors; host mismatch by the existing `ssh.FixedHostKey` callback result, **never by bypassing pinning**; authentication failure using only a known crypto/ssh error category. Classify SFTP not-found/permission statuses without exposing their raw server messages. Missing/symlinked/non-directory remote paths must remain fail-closed.
3. Preserve categorized errors from the dialer through `CreateSSHRemoteSandbox` (bootstrap) and the desktop Wails binding. HTTP returns 503 with an allowlisted code `ssh_remote_<stage>` and sanitized message. Other domain, policy and validation errors keep existing status semantics. No new credential or SSH management APIs.
4. Add a **scoped SSH creation error presenter**, used only by the SSH Sandbox wizard, with localized Chinese/English per-stage cause + operator next step. Support both HTTP `MenderApiError.code` and the Wails safe `ssh_remote_failure:<code>` string (Error or rejected string). Never present untrusted backend error text verbatim. Unknown failures show a useful safe generic SSH message, not "Unexpected error"; existing non-SSH flows remain unchanged.
5. Root login stays permitted with a nonblocking high-privilege warning. Preserve explicit pinned host identity and RSA-SHA2-only signatures, Policy/Principal/Lease, bounded connection limits, SFTP/path restrictions, audit and no local-host fallback. No automatic remote directory creation or host-key scanning/TOFU.
6. Tests: typed-code redaction, localhost refused connection, generated loopback SSH server with a deliberately wrong host pin, rejected login, SFTP disabled, missing/invalid workspace and success; API 503 sanitized code; frontend HTTP/Wails message mapping and no key leaks. Never read OS Keychain entries, call a user-owned IP, or persist remote resources for negative tests.

## Delivery
A. This contract committed before code.
B. Backend typed stages + tests + HTTP propagation committed separately.
C. Frontend localized stage-specific feedback + tests, followed by full `pnpm check` and closure evidence. Exclude `.agents/`, `.pi/`, `.intent-loop/` from all commits.

## Operator explanation after repair
When the user retries, show the **actual stage observed**. If it is a host-key mismatch, direct them to verify the host public key on the trusted server (e.g. `/etc/ssh/ssh_host_ed25519_key.pub`, or the corresponding RSA/ECDSA host key), which is **not** the SSH login credential's public key. If it is authentication, verify key authorization for the selected login account. If workspace missing, check the existing remote directory. Do not ask for private key contents.
