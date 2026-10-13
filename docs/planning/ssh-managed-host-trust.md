# SSH Remote: managed host identity instead of mandatory manual public-key form

Status: **Closed (2026-10-13)**. This supersedes the manual pinned-host-key UI contract in Phase 32 and the focused SSH provision diagnostics. Document-first, incremental commits, no compatibility bridge.

## Operator problem
- SSH login private keys authenticate a user; SSH host public keys authenticate the **server**. Cloud-provided account key pairs and uploaded SSH public keys do **not** identify the SSH server. Copying an account `id_rsa.pub` into Mender's "Pinned SSH host public key" field causes a legitimate pinned-host mismatch even if interactive `ssh -i` login succeeds.
- Forcing users to locate and paste a host key is a needlessly brittle onboarding flow. SSH clients commonly read an existing `known_hosts`, or prompt to verify a fingerprint on a first connection.

## Contract
1. SSH Sandbox configuration shows only name, host, port, login user (root allowed with advisory), workspace path and SSH private-key credential. **Do not require or display a host public-key textarea.** Keep a technical preview of the host's SHA256 fingerprint on the first-connection trust dialog, not a full raw key entry. Existing CLI/API callers supplying explicit pins remain supported; no adapter or legacy UI.
2. Implement a scoped, local-operator-only `InspectSSHHostKey(host,port)` that performs a bounded **pre-auth** SSH host-key handshake, validates the server's cryptographic SSH host key and returns a stable public-key fingerprint, algorithm, safe trust status `trusted | untrusted | changed`, plus the candidate public host key for exact pinning during provisioning. No SSH private key or user login is used for this preview.
3. Trust sources: read existing user `~/.ssh/known_hosts` (including OpenSSH hashed and nonstandard-port hostnames) **and** Mender's own `ssh_known_hosts` kept alongside the Runtime database. Do not edit the user's `~/.ssh/known_hosts` file. Absent files mean unknown; malformed files fail closed. Known trust sources that disagree must not be overridden by a match in a different file. Explicit host-identity changes must block, not automatically update.
4. With trusted identity, Review/Create can proceed automatically, passing an exact pinned public key internally. With an unknown host, require a deliberate **first-connection fingerprint confirmation**; show SHA256 fingerprint and instruct independent verification by a trusted channel. Do not silently trust a scanned key or bypass `ssh.FixedHostKey`. If the key changes between preview/confirmation/creation, the pinned handshake must fail. A changed trusted host key is blocked with a distinct message and no one-click override.
5. After an admitted, successful pinned SSH + SFTP + workspace verification, persist that exact host public key in Mender-managed host trust for future sessions; use secure file permissions and reject managed-file symlinks, conflicts, or corruption. The pinned key continues to be stored in the immutable SSH Sandbox template for **all subsequent execution and lifecycle**.
6. Backends must gate host probing to the local operator and `sandbox.provision` admission **before network access**. No model/MCP/Provider access to arbitrary probes; no untrusted origin, SSH agent forwarding, raw key leaks or insecure host-key callback. Preflight never creates a Sandbox or connects to arbitrary user-controlled hosts in automated tests.
7. Preserve RSA SHA-2 signatures (no SHA-1 `ssh-rsa`), Ed25519/ECDSA, RSA >=2048, private key Keychain protection, error-stage taxonomy and governance. Root login remains permitted with a non-blocking privilege warning.
8. Update HTTP/OpenAPI, SDK, Wails binding and React together. Add local test-server regressions for trusted/untrusted/changed server keys, system known_hosts, hashed hosts, exact pinned creation, first-time dialog accept/cancel, missing credentials, server key rotation, no remote real hosts, API/local-owner checks and full `pnpm check`.

## Delivery
A. Commit this contract.
B. Implement host identity inspector and Mender-owned trust source with transport tests; commit.
C. Implement local-authorized API/Wails/SDK exposure, adjust frontend wizard and i18n, add UI tests, commit by sub-boundary.
D. Run full `pnpm check`, native desktop Go build, close document and commit. Ignore `.agents/`, `.pi/`, `.intent-loop/`.

## Closure and acceptance (2026-10-13)

Status: **Closed**. No real Rocky/third-party SSH server was contacted. The previous manual host public-key form is removed; user-supplied SSH login credentials are unchanged.

### Delivered commits
- `4a92354`: document-first host trust contract and explicit no-silent-TOFU boundary.
- `2d0e8c0`: pre-auth cryptographic SSH host identity inspection, OpenSSH known_hosts and separate Mender-managed trust, safe mismatch detection and immutable pinned sandbox connection, tests for first-use/trusted/changed/hashed-host entries and trust-file symlinks; store exact verified host key only following successful admitted SSH/SFTP connection.
- `dd85283`: local-operator and Sandbox Policy admitted host inspection through management HTTP, typed OpenAPI/SDK, Wails bindings, local-authority tests and generated schema.
- `32ecd89`: drop mandatory host-key textarea from SSH creation, detect known trust automatically, first-use SHA256 fingerprint confirmation dialog (cancel means no provision), block host-key changes, localized Chinese/English guidance and UI/Wails tests.

### Evidence
- Test-owned loopback SSH/SFTP plus Sandbox admission/router/lifecycle demonstrates new/untrusted discovery, exact pinned SSH/SFTP provisioning and later known trust, without touching real host credentials.
- Unit tests cover hashed `known_hosts`, 0600 Mender trust file, symlink rejection, changed host key blocking, no trust write during pre-auth inspect, cancelled confirmation, trusted auto-provision, and no creation without a credential. The existing SSH RSA-SHA2 / host pinning and provisioning diagnostics tests remain intact.
- `pnpm check` passed, including Go lint/tests/race, plugin SDK/example checks, OpenAPI/SDK/Wails binding parity, strict TS and ESLint, i18n (1167/1167 keys), **202/202 frontend tests**, and Web/Desktop production builds. `GOWORK=off go build ./...` in `apps/desktop` passed with existing macOS linker deployment-target warnings. The temporary native Go executable was removed.
- The running Mender Wails dev app, tunnel and OS keychain entries were **not restarted, read, or changed**. `.agents/`, `.pi/` and `.intent-loop/` remain untouched and uncommitted.

### Operator use
Restart the Wails dev runtime to load the new Go API and frontend. Configure host, port, user, remote workspace and existing SSH key credential; no server host key entry. On first use, Mender displays the SHA256 fingerprint returned in the SSH server-signed handshake; compare it independently (e.g. via SSH server console `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub`) before clicking **I verified the fingerprint**. A trusted host skips this prompt. If trust changes, Mender blocks until the operator verifies and repairs the trust record; it does not automatically replace trusted keys. The server may still reject root login or lack the specified workspace; existing SSH error diagnostics will explain such failures.
