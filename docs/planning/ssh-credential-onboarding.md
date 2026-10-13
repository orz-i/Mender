# SSH credential onboarding — compatibility without weakening trust (2026-10-12)

Status: **Closed (2026-10-12)**. Document-first follow-up to Phase 32 (not a new Phase or compatibility bridge).

## Reproduction and diagnosis
- SSH credentials reject valid RSA 2048 private keys with a blanket RSA >=3072 restriction, even though the default Go SSH stack supports RSA SHA-2 signatures. Do **not** re-enable SHA-1 ssh-rsa authentication or DSA.
- A passphrase-protected key is rejected at import instead of offering passphrase entry. The current SSH form requires a disabled namespace, two names, a reference preview, and manually pasting key material.
- SSH host pinning accepts only Ed25519, despite other supported strong host-key formats. **Pinned and verified host identity remains mandatory.**
- Generic frontend errors obscure whether a key is malformed, weak or encrypted.

## Contract
1. Accept Ed25519, ECDSA P-256/384/521 and RSA >=2048 **login** keys. Reject DSA and RSA <2048. No weak SHA-1 signature negotiation. Validation and normalization happen server-side on creation **and** rotation. Keep native OS keyring SSH-only, unchanged provider/plugin/SQLite exposure boundaries.
2. Optional import-time passphrase is write-only, accepted only on SSH key create/rotate. Parse encrypted PEM/OpenSSH, store an unencrypted normalized OpenSSH key in the **existing OS keychain** (already used for plaintext keys). Do **not** store passphrase, return it, serialize it to metadata, persist it to disk, or require it on each connection. UI must disclose that the OS keychain protects the imported key. Wrong/missing passphrases receive safe, non-secret diagnostics.
3. Accept exactly one pinned OpenSSH Ed25519/ECDSA or RSA >=2048 **host** public key; preserve fixed host-key checking (no TOFU/insecure callback). Explicit pin remains required. Reject DSA, RSA <2048, authorized_keys options/additional lines, unrecognized formats.
4. SSH form asks only for a user-facing credential name and a key (paste or local file). Mender generates a normalized credential reference and display name without duplicate namespace/name/reference fields. Show optional passphrase and unobtrusive supported-format advice, clear errors; no key material in notifications or logs. Generic Credential form is unchanged.
5. Backend remains authoritative; client validation is ergonomic only. Preserve RPC/HTTP behavior for non-SSH secrets and optional/absent passphrase.
6. Tests: RSA 2048/1024, Ed25519, ECDSA, encrypted PEM/OpenSSH, wrong/missing passphrase, host pin variations and one-key restrictions, create/rotation keyring content, typed HTTP/Wails/SDK, frontend import and localized error. Only generated ephemeral keys; never user-owned credentials.

## Non-goals
No password auth, ssh-agent, hardware keys, host-key bypass, host-key auto-import, root-login relaxation, remote isolation claims, or user-owned SSH server access in tests.

## Delivery
A. Contract + regressions and backend key import/host pinning.
B. HTTP/Wails/SDK schema parity + focused SSH credential/Sandbox UI.
C. Go/TypeScript tests and static checks; final security review and acceptance evidence.

## Acceptance evidence
- `c22f99f`: document-first compatibility and security boundaries.
- `68e9149`: normalized SSH key import (RSA >=2048/Ed25519/ECDSA, encrypted PEM/OpenSSH, import-only passphrase), RSA-SHA2 pinned host and login signature selection, typed HTTP/OpenAPI/SDK/Wails propagation.
- `db36c1b`: in-process RSA 2048 / SHA-2 end-to-end SSH/SFTP handshake, HTTP invalid-key 400 rather than 500, secure error redaction, native-keyring rotation tests.
- `9e98aee`: minimal SSH create/rotate UI, file picker with hidden contents, optional passphrase and localized diagnostics, SSH host key alternatives, frontend/Wails regression tests.
- Final alignment: historical Phase 32 rules explicitly superseded; CLI host pin help and OpenAPI hostKey length/description corrected; test-only lint exceptions narrowly documented.
- Verification: `go test ./backend/internal/credential/... ./backend/internal/sandbox/infrastructure/sshremote ./backend/internal/management/httpapi` passes; 42 targeted UI+Wails tests pass, `pnpm i18n:check` covers all 1135 messages; `pnpm check` passed for provider/sandbox/tunnel SDKs, Go lint/tests/race, SDK/Wails generation, TypeScript lint/typecheck, 192 frontend tests and Web/Desktop builds. Final generated contract parity is rechecked after the last documentation/alignment commit.
- No real SSH private key or user-owned SSH host used for tests. No raw key in persistent metadata, diagnostics or plugin configurations. SHA-1 RSA signatures, weak keys, silent host trust and root logins remain blocked. The only retained unencrypted normalized key material is in the existing native OS credential store; passphrase never persists.
