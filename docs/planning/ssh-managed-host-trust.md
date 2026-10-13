# SSH Remote: managed host identity instead of mandatory manual public-key form

Status: **in progress** (2026-10-13). This supersedes the manual pinned-host-key UI contract in Phase 32 and the focused SSH provision diagnostics. Document-first, incremental commits, no compatibility bridge.

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
