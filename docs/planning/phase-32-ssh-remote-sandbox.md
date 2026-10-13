# Phase 32 — SSH Remote Sandbox plugin and SSH credential

Status: **Closed (2026-10-11)**, following Phase 31. Document-first, incremental commits, no compatibility bridge.

**Superseded SSH key-format restrictions (2026-10-12):** This document preserves the original v1 acceptance decisions. The follow-up [`ssh-credential-onboarding.md`](ssh-credential-onboarding.md) supersedes the unencrypted-key and Ed25519-only host-key constraints: Mender now supports Ed25519/ECDSA/RSA >=2048, import-time passphrases, and pinned RSA-SHA2/ECDSA host keys. Explicit host pinning and the other execution trust boundaries remain mandatory.

## Audited constraints
- Mender's unified `sandbox.backend` Registry/Execution Router already applies Principal, Policy, durable Lease, Audit, and exact BackendBinding.
- Third-party native Sandbox plugins explicitly reject Credential References (`InspectConfig.credential_refs`), and sandbox configs reject raw private keys. Reusing that path for SSH would either leak private keys into an untrusted process or weaken the guard: prohibited.
- Mender already has native Keychain-backed Credential metadata + secret resolver. There is currently no SSH credential kind or SSH Sandbox. Existing `IsolationRemote` is only a transport label, **not proof of a chroot/VM/container sandbox**.
- Linux/Unix remote servers implement SSH and commonly SFTP; use `golang.org/x/crypto/ssh` and `github.com/pkg/sftp`, not host `ssh` subprocesses, SSH agent forwarding, `DOCKER_HOST` or permissive host-key callbacks. See https://pkg.go.dev/golang.org/x/crypto/ssh and https://pkg.go.dev/github.com/pkg/sftp .

## Contract
1. Ship first-party `io.mender.sandbox.ssh-remote@builtin`, kind `sandbox.backend/main`. It is always discoverable and **configurable**, not live-connected by browsing Marketplace. Create route `/sandboxes/new?pluginId=io.mender.sandbox.ssh-remote`; no second marketplace. UI makes remote host/control scope explicit.
2. Auth is a dedicated **SSH private key** Credential selection, reference `credential://ssh/<name>`. The private key is stored by existing OS native keyring, never SQLite, plugin config, descriptor, tools, audit or API response. Validate unencrypted SSH private key server-side on create/rotate and resolve only at connection time. No password/agent forwarding. Private-key-with-passphrase and hardware agents can follow another reviewed phase.
3. Require explicit **pinned host public key** (OpenSSH authorized-key line) and fail closed on changes via `ssh.FixedHostKey`. No TOFU, `InsecureIgnoreHostKey`, optional missing host-key check, automatically fetched trust, ProxyCommand or unsigned host key. Explicit host, port, login user, POSIX absolute workspace root are immutable and reviewed.
4. Connection/provision: admit owner `sandbox.provision` **before** network/key resolution; verify SSH handshake, pinned host key, login, and SFTP root, then atomically persist ready Sandbox + immutable nonsecret template. Creation does not install binaries or create/modify remote resources.
5. Read/search/edit operate via bounded SFTP using relative paths, explicit symlink checks, and no host filesystem fallback. Remote shell executes as exactly the configured SSH login on that host; it may access anything the account can access. **Do not claim strong filesystem/sandbox isolation**: a privileged remote account and shell access can escape the displayed workspace. Protect against accidental path traversal, bound time, number of files, bytes, stdout/stderr, and reject unsafe remote target features. All actions remain under caller Policy and Lease.
6. Lifecycle stop/start/destroy is **local access control** only: stop disables acquisitions, start re-verifies the remote credentials/host key/root and advances Generation, destroy tombstones the local binding. No remote server reboot, delete or container shutdown. Deny destruction with any active lease / Workspace Provider reference. Durable events audited.
7. Deleting an SSH Credential must be blocked while referenced by any non-destroyed remote Sandbox, even when unreachable. Credential rotation requires validation; established session ownership and no secret exposure.
8. HTTP/OpenAPI SDK, native Wails, React creation/credentials flows, Marketplace status, i18n parity and CLI/management tests. No remote endpoint provided by user: acceptance on deterministic in-process SSH/SFTP test server, with optional user-owned remote smoke only after separate host details are supplied. Do not create a remote resource against an unknown address.

## Commits
A: this contract.
B: SSH key Credential validation, pinned-host transport and SFTP/Shell implementation with isolated tests.
C: typed Sandbox binding + SQLite schema v11, credential deletion guards, lifecycle, Marketplace.
D: HTTP/Wails/SDK/React credential and focused Sandbox wizard, full tests and closure.

Excluded: auto-SSH-discovery, exposing raw passwords/private keys to plugins, automatic port forwarding, keyscan trust, remote Docker bootstrap, arbitrary local ssh config, VM isolation claims, remote cleanup, user-owned test hosts. Ignore `.agents/`, `.pi/`, `.intent-loop/`.


## Closure & acceptance (2026-10-11)

**Status: Closed.** SSH Remote is an in-process, first-party
`sandbox.backend/main` plugin, *not* a third-party process with private-key
access and not a new Agent loop.

### Operator usage

1. Credentials → Add Credential → **SSH private key**; enter a name and an
   **unencrypted** private key. The fixed namespace is `ssh`, producing
   `credential://ssh/<name>`. Keys are validated before an OS-keyring write,
   rotated only through credential rotation and never returned by management
   APIs. Historical SSH environment-backed credentials fail closed.
2. Plugin Marketplace → **SSH Remote** → **Create sandbox**. Enter host, port
   (default 22), unprivileged login user, canonical absolute POSIX workspace,
   the explicitly reviewed `ssh-ed25519` host **public** key and select the
   existing `credential://ssh/...` reference. Review the **remote account,
   not isolated** warning before creation. The form never accepts or sends a
   private key.
3. Once admitted by Sandbox Policy, the built-in uses SSH host-key checking
   and SFTP to verify the remote workspace, then atomically persists only
   the nonsecret binding and a ready Run. The Workspace Provider's Sandbox
   selector now lists ready SSH resources and carries the non-isolation warning.
   Sandbox details support local Stop/Start/Destroy; Start re-verifies SSH,
   Stop/Destroy never modify remote service/files and active durable Leases or
   Workspace Provider references prevent destructive transitions.
4. CLI parity: `mender sandbox add ssh-remote --name ... --host ... --port 22
   --user ... --root ... --host-public-key 'ssh-ed25519 ...'
   --credential-ref credential://ssh/<name>`. The CLI does not accept private
   key bytes, SSH agents or password flags.

### Security and operational limits

- The pinned host key is mandatory; changing its value fails authentication.
  No TOFU, host-key bypass, known-hosts auto-import, arbitrary local SSH
  config, jump host, `ProxyCommand`, TCP tunnel or privilege elevation.
- SSH is not a container, VM, chroot, network sandbox or shell filesystem
  confinement. Shell acts as the remote login, which may have other filesystem
  or sudo rights configured by the operator. Only a dedicated least-privilege
  remote user on a trusted host is appropriate.
- Structured SFTP paths reject explicit parent traversal/symlinks, but cannot
  guarantee race-free containment against another process concurrently
  changing the remote filesystem. File create requires SFTP exclusive-create;
  replace requires the POSIX rename extension (no unsafe fallback).
- Shell and structured operations have bounded inputs/results, SSH connection
  deadlines, timeout-driven transport cancellation and uncertain-session
  durable Lease retention. Directory traversal has file/entry/directory/depth
  budgets. A failed remote action never reroutes to local host tools.
- Private keys are resolved only after Policy/Lease authorization, copied for
  the connection, and no raw secret enters SQLite, the Plugin catalogue,
  audits, remote templates or normal Tool arguments.
- An SSH Credential with a non-destroyed Sandbox binding cannot be deleted,
  including when the remote host is offline. A destroyed tombstone remains in
  inventory and no remote path is deleted automatically.
- SSH login key: unencrypted Ed25519/ECDSA or RSA >=3072; host public key:
  pinned `ssh-ed25519`. Passphrase-protected keys and hardware security keys
  are intentionally not implemented in v1.

### Verifications

- `pnpm check` passed: Go lint, Go tests and Race, SDK/OpenAPI/Wails binding
  consistency, TypeScript strict typecheck, ESLint, **980/980 zh-CN strings**,
  **164/164 Vitest tests** and Web/Desktop production builds.
- Native macOS Wails `GOWORK=off go build ./...` succeeded. Existing linker
  deployment-target warnings remain (not introduced by this feature).
- `pnpm security:check` passed for Go and production JS. The report notes
  one **module-only** advisory `GO-2026-5932` involving the unmaintained
  `golang.org/x/crypto/openpgp` package, **not imported or called by
  Mender's SSH backend**; zero reached vulnerable symbols/packages.
- Live-protocol testing uses a **test-owned loopback SSH/SFTP server**, a
  generated ephemeral login key, pinned server public key, real read/search/
  edit/shell, mismatch denial, atomic-exclusive-create, symlink denial,
  Policy → Registry → Execution Router → durable Lease → Run → lifecycle →
  SQLite v11 readback, with `-race`. No user-owned SSH target or OS Keychain
  record was read/written during these tests.
- Forward-only Sandbox SQLite v10 → v11 migration independently verified:
  one consistent pre-upgrade `.mender-sandbox-v10-backup-*.db`, stable existing
  Sandbox IDs/Runs and no duplicate snapshots on restart.
- Negative gates: wrong host key, invalid key/credential, forbidden user/host,
  Service/MCP-initiated resource creation, remote disconnection and failed
  SSH CLI/API creation do not register a remote Sandbox.

### History

- `7cfc844` — document-first intent, boundary and acceptance
- `3404d28` — SSH and Keychain/SQLite execution core
- `8adeadb` — typed management HTTP/Wails/SDK interfaces
- `d8d3a08` — SSH key credential and guided creation UI
- `ab15441` — E2E Router/identity tests, native-only key fence,
  exclusive SFTP create and v10 migration snapshot
- `d8d8a16` — CLI parity, Workspace Provider selector and resource lifecycle UX
- Final closure commit — this documentation and final dev-app smoke.

### Not claimed

No user's real remote host was exercised or configured; that requires the
operator to supply a host, valid private-key Credential, pinned host public key
and non-root account. Native production app signing and packaged release QA
remain separate, and remote host trust/isolation remains the operator's
responsibility. No modifications to `.agents/`, `.pi/` or `.intent-loop/`.
