# Phase 32 — SSH Remote Sandbox plugin and SSH credential

Status: In progress (2026-10-11), following Phase 31. Document-first, incremental commits, no compatibility bridge.

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
