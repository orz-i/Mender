# SSH Sandbox review validation feedback (2026-10-13)

Status: In progress. Focused follow-up to Phase 32 and SSH credential onboarding; no new compatibility bridge.

## Reproduction
The create-SSH-Sandbox wizard disables Review using one combined `sshValid` boolean with no field-level diagnostics. A form using `root` and a host-key payload missing its OpenSSH algorithm header cannot proceed, yet neither field explains why. The backend rejects root deliberately; pinned server identity must contain exactly one valid supported OpenSSH host public key. The screenshot is not evidence that the credential itself is invalid.

## UX contract
1. Review is always actionable in SSH configuration mode: clicking it attempts client-side validation without provisioning or reaching the SSH server. Valid input advances to the review step; invalid input displays localized, actionable field errors and a compact reason summary adjacent to Review. Errors update while editing. No silent disabled button.
2. Proactively explain the two common mistakes when encountered: `root` is disallowed by the existing SSH Remote security boundary; a host-key Base64 fragment without algorithm prefix is not a complete public key. Distinguish `known_hosts` hostname-prefix lines and multiple lines, and show how to obtain the **public host key** from a trusted server (e.g. `/etc/ssh/ssh_host_ed25519_key.pub`). Avoid copying private keys or putting any host-key payload in diagnostics.
3. Keep the screen focused on configuration, not a second sprawling validation form. Individual errors belong below their field; after Review fails, the compact footer summary makes the blocker visible without scrolling.
4. SSH client validation should agree with backend on common constraints: hostname/IP syntax, port 1..65535, account identifier, non-root, absolute canonical POSIX root other than '/', single pinned Ed25519/ECDSA/RSA >=2048 public-key line, and existing native SSH Credential selection. A trailing newline from a pasted public-key line is harmless and should be trimmed before validation/submission.
5. Client hints are not authority. Go `Template.Validate`, cryptographic key parsing, `ssh.FixedHostKey`, Principal/Policy/Lease, and remote SSH handshake remain unchanged. Do **not** enable root automatically, skip host pinning, trust SSH keyscan unverified, or create real remote resources during tests. Changing root access requires a separate explicit privileged-mode design and authorization.
6. Regression coverage for the exact screenshot-like `root` + key-fragment blockers, other invalid fields, correct typed pins and newline handling, recovery to Review, and i18n/strict TS. Keep test fixture keys synthetic.

## Delivery
A. This diagnosis and UX contract, committed first.
B. Extract reusable SSH validation, wire field/error feedback, update zh-CN, and add interaction tests; commit.
C. Full `pnpm check` and closure notes, final commit. Never add `.agents/`, `.pi/`, or `.intent-loop/` to Git.
