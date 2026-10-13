# SSH Remote provisioning diagnostics — safe errors end to end (2026-10-13)

Status: **Closed (2026-10-13)**. Follow-up to Phase 32 and root-login advisory closure; document first, incremental commits, no compatibility bridge.

**Updated user flow:** [`ssh-managed-host-trust.md`](ssh-managed-host-trust.md) supersedes the manual host key entry requirement. Pre-auth inspection reuses trusted known_hosts; first-use fingerprint confirmation pins the server host identity. The failure-stage codes and diagnostics defined here remain applicable.

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

## Closure and acceptance (2026-10-13)

**Status: Closed.** The real Rocky server was not contacted in this phase, so the original concrete server-side cause is still undetermined. This patch makes the next failed attempt report an actionable, safe failure stage instead of a generic error; it does **not** claim that the remote connection now succeeds.

### Commits
- `6b78604` — document-first SSH provisioning error contract and data-minimization boundaries.
- `a60b06c` — typed first-party SSH failure stages in transport, preserved through Bootstrap, HTTP 503 reason codes and Wails error strings; localhost handshake/SFTP failure injection and redaction tests.
- `ec0cc56` — scoped SSH creation error presenter, Chinese/English messages, Web/Wails/HTTP error-code mapping, recovery suggestions and UI regression tests.

### Verified behavior
- The SSH backend classifies: credentials unavailable/invalid, network refused/timeout/unreachable, pinned host identity mismatch, authentication rejected, handshake failure, SFTP unavailable, workspace missing/access denied/invalid. All messages are allowlisted fixed codes and still satisfy `errors.Is(err, sandbox.ErrBackendUnavailable)`.
- Test-owned SSH/SFTP server verifies the actual post-handshake stages for host-key mismatch, rejected public-key login, disabled SFTP, missing workspace, non-directory workspace and symlink ancestry. Network connection-refused and returned-SFTP-error classification tests pass. No raw public/private key, username, host, remote path or remote library message is reflected in the serialized failure.
- HTTP handler returns sanitized stage-specific `ssh_remote_<code>` errors without changing unrelated policy/validation classifications. Wails retains typed code through its safe string rejection; UI recognizes only allowed codes and translates each into a safe operational suggestion, never dumps arbitrary remote error messages.
- `pnpm check` **passed**: Go SDK and examples, Go lint (0 issues), Go tests, Go race, SDK/OpenAPI/Wails generated binding checks, TypeScript strict checks, ESLint, i18n (1163/1163), **202/202 Vitest tests**, Web/Desktop frontend builds. Native `GOWORK=off go build ./...` in `apps/desktop` also passed; existing macOS deployment-target linker warnings are unrelated to this change. The temporary standalone build binary was removed.
- The running Wails development app and SSH/Tunnel connections were not restarted or modified. No user SSH private-key credential was inspected, no real SSH host contacted and no remote files/resources were created.

### Operator next step
Restart or let Wails rebuild the Go backend, retry the same SSH Sandbox configuration, and read the **new specific error**. If it says `host_key_mismatch`, verify the **server** host public key independently (for RSA typically `/etc/ssh/ssh_host_rsa_key.pub`) rather than using the account login key's `id_rsa.pub`. If it says `authentication_failed`, check the selected account's `authorized_keys` and server root public-key policy. If it says `workspace_missing`, check the existing remote `/data/workspace` directory. Never paste private key contents into bug reports.

## Operator explanation after repair
When the user retries, show the **actual stage observed**. If it is a host-key mismatch, direct them to verify the host public key on the trusted server (e.g. `/etc/ssh/ssh_host_ed25519_key.pub`, or the corresponding RSA/ECDSA host key), which is **not** the SSH login credential's public key. If it is authentication, verify key authorization for the selected login account. If workspace missing, check the existing remote directory. Do not ask for private key contents.
