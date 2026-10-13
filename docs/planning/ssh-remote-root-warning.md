# SSH Remote root-login warning instead of rejection (2026-10-13)

Status: **Closed (2026-10-13)**. Follow-up to the focused [SSH Sandbox review validation](ssh-sandbox-review-validation.md) contract.

## User request and decision
The operator explicitly requested that SSH Remote accept the `root` SSH login user, with a **warning only**. A remote SSH Workspace is a transport into the specified account, not an OS/container/VM isolation boundary. Root can read, write and execute across the remote host within its authentication and OS policy. Hard-rejecting a valid user name does not provide isolation.

## Implementation contract
1. Remove the special `root` rejection **in both** the Go `sshremote.Template.Validate` and client-side `validateSSHRemoteReview`; the same login syntax applies to `root` as to other accounts. Preserve all host, port, canonical root, credential-reference and pinned host-key checks.
2. When `user === "root"`, show a short, localized high-privilege warning directly beneath the login field **and again on authority review**. The warning is advisory: no extra checkbox, extra step, confirmation dialog, hidden deny, or disabled Review/Create. Other validation issues remain blocking and continue to show actionable feedback.
3. Keep Principal/Policy/Lease, audit, trusted host-key verification and existing SSH transport signature restrictions unchanged. Do not imply filesystem isolation. The SSH server remains authoritative about whether that account is permitted to authenticate.
4. Update CLI help, old planning documents (as historical superseded constraints), bilingual messages and regression coverage. Verify non-root behavior remains unchanged; generated SSH test server may simulate a root login. Do not access an operator-owned host or key in tests.
5. Document-first, sequential commits; exclude `.agents/`, `.pi/`, `.intent-loop/` from Git.

## Implementation batches
A. Amend this contract, commit.
B. Remove Go/frontend root block, surface configuration and review warnings, extend tests and CLI help, commit.
C. Run targeted/full gates, update acceptance evidence, commit closure.

## Closure / verification (2026-10-13)
- `de3c8b6` — document-first intent and explicit advisory-only root policy.
- `cf50abf` — Go and React validation allow `root` while retaining strict host pinning, credential handling and other governance; warning appears on both configuration and review, without an extra confirmation; CLI help and non-root messaging updated.
- `e7eadc6` — update the older Plugin Marketplace Review test to assert the current actionable (not silently disabled) button behavior.
- Synthetic loopback SSH/SFTP server authenticated a test login named `root` without OS privilege escalation or any user-owned host access. HTTP acceptance of the root login passes validation, then fails closed with HTTP 503 on missing Keychain credentials; no remote Sandbox is persisted. CLI negative test confirms the same fail-closed behavior.
- `pnpm check` **passed**: backend Go lint/tests/race, all plugin SDK/example gates, OpenAPI/SDK/Wails bindings and desktop architecture checks, ESLint, strict TypeScript, all 1148 zh-CN translations, **198/198 frontend tests**, and Web/Desktop builds. Existing bundle-size warnings are unrelated to SSH.
- Current SSH Remote `root` login is allowed but not isolated: permission to authenticate remains on the remote server, and remote commands inherit that account's actual OS rights. Explicit host key pinning, Principal/Policy/Lease and audit remain required. The remote workspace root directory must still be a normalized absolute non-`/` path.
